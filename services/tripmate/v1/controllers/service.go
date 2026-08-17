package controllers

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	apphash "github.com/jblabs/tripmate-be/pkg/hash"
	appjwt "github.com/jblabs/tripmate-be/pkg/jwt"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	googleoauth "github.com/jblabs/tripmate-be/pkg/oauth/google"
	"github.com/jblabs/tripmate-be/pkg/response"
	authcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/auth"
	expensecontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/expense"
	expensecategorycontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/expense_category"
	financecontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/finance"
	invitationcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/invitation"
	participantcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/participant"
	receiptcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/receipt"
	tripcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/trip"
	usercontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/user"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	ratesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/exchange_rates"
	categoriesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_categories"
	payersdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_payers"
	splitsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_splits"
	expensesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expenses"
	outboxdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/outbox_events"
	receiptsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/receipts"
	refreshtokens "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/refresh_tokens"
	settlementsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/settlements"
	invitesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/trip_invitations"
	partsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/trip_participants"
	tripsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/trips"
	users "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/users"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	expensecategorydomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense_category"
	finaldomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/finalization"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	invitationdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/invitation"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	userdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/user"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB      *gorm.DB
	Cfg     *config.Config
	Log     *slog.Logger
	Storage receiptdomain.ObjectStorage
	OCR     receiptdomain.OCRProvider
}

type Service struct {
	auth       authcontroller.Controller
	users      usercontroller.Controller
	trips      tripcontroller.Controller
	parts      participantcontroller.Controller
	invites    invitationcontroller.Controller
	expenses   expensecontroller.Controller
	categories expensecategorycontroller.Controller
	finance    financecontroller.Controller
	receipts   receiptcontroller.Controller
	issuer     *appjwt.Issuer
}

func NewService(deps Dependencies) *Service {
	issuer := appjwt.NewIssuer(appjwt.Config{
		AccessSecret: deps.Cfg.JWT.AccessSecret, RefreshSecret: deps.Cfg.JWT.RefreshSecret,
		AccessTTL: deps.Cfg.JWT.AccessTTL, RefreshTTL: deps.Cfg.JWT.RefreshTTL,
	})
	inviteRepo := invitesdb.New(deps.DB)
	var googleVerifier userdomain.GoogleVerifier
	if deps.Cfg.Google.ClientID != "" {
		googleVerifier = googleVerifierAdapter{v: googleoauth.NewVerifier(deps.Cfg.Google.ClientID)}
	}
	userService := userdomain.NewService(userdomain.Dependencies{
		Repo:   users.NewGormPostgresqlAdapter(deps.DB),
		Tokens: refreshtokens.NewGormPostgresqlAdapter(deps.DB),
		Hasher: apphash.NewArgon2Hasher(), Issuer: issuer, Invitations: inviteRepo, Google: googleVerifier,
	})
	tripRepo := tripsdb.New(deps.DB)
	partRepo := partsdb.New(deps.DB)
	expenseRepo := expensesdb.New(deps.DB)
	settlementRepo := settlementsdb.New(deps.DB)
	receiptRepo := receiptsdb.New(deps.DB)
	categoryRepo := categoriesdb.New(deps.DB)
	rateService := fxdomain.NewService(fxdomain.Dependencies{Repo: ratesdb.New(deps.DB), UOW: appdb.NewGormUnitOfWork(deps.DB)})
	tripService := tripdomain.NewService(tripdomain.Dependencies{Repo: tripRepo, Participants: partRepo, Tx: tripTransactor{deps.DB}, Expenses: expenseRepo, FX: rateService})
	partService := participantdomain.NewService(partRepo, tripRepo, participantdomain.CompositeActivityCounter{expenseRepo, settlementRepo})
	categoryService := expensecategorydomain.NewService(categoryRepo)
	expenseService := expensedomain.NewService(expensedomain.Dependencies{
		Expenses: expenseRepo, Payers: payersdb.New(deps.DB), Splits: splitsdb.New(deps.DB),
		Participants: partRepo, Outbox: outboxdb.New(deps.DB), UOW: appdb.NewGormUnitOfWork(deps.DB), Receipts: receiptRepo,
		Categories: categoryRepo,
	})
	receiptService := receiptdomain.NewService(receiptdomain.Dependencies{Repo: receiptRepo, Participants: partRepo,
		Storage: deps.Storage, OCR: deps.OCR, Expenses: expenseService, UOW: appdb.NewGormUnitOfWork(deps.DB)})
	balanceService := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRepo, Settlements: settlementRepo, Participants: partRepo, FX: rateService})
	settlementService := settlementdomain.NewService(settlementdomain.Dependencies{Repo: settlementRepo, Participants: partRepo, Outbox: outboxdb.New(deps.DB), UOW: appdb.NewGormUnitOfWork(deps.DB)})
	finalService := finaldomain.NewService(finaldomain.Dependencies{Balances: balanceService, FX: rateService, Trips: tripRepo, Outbox: outboxdb.New(deps.DB), UOW: appdb.NewGormUnitOfWork(deps.DB)})
	inviteService := invitationdomain.NewService(inviteRepo, tripRepo, userService, partService)
	return &Service{
		auth: authcontroller.NewController(userService), users: usercontroller.NewController(userService),
		trips:   tripcontroller.NewController(tripService, partService),
		parts:   participantcontroller.NewController(tripService, partService),
		invites: invitationcontroller.NewController(tripService, partService, inviteService), issuer: issuer,
		expenses:   expensecontroller.NewController(tripService, partService, expenseService),
		categories: expensecategorycontroller.NewController(tripService, partService, categoryService),
		finance:    financecontroller.NewController(tripService, partService, balanceService, settlementService, rateService, finalService),
		receipts:   receiptcontroller.NewController(tripService, partService, receiptService),
	}
}

func (s *Service) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/ping", s.Ping)
	s.auth.RegisterRoutes(group)
	protected := group.Group("")
	protected.Use(middleware.Authenticate(s.issuer))
	s.users.RegisterRoutes(protected)
	s.trips.RegisterRoutes(protected)
	s.parts.RegisterRoutes(protected)
	s.invites.RegisterRoutes(protected)
	s.expenses.RegisterRoutes(protected)
	s.categories.RegisterRoutes(protected)
	s.finance.RegisterRoutes(protected)
	s.receipts.RegisterRoutes(protected)
}

// googleVerifierAdapter adapts pkg/oauth/google's Verifier (which returns its own Claims type) to
// the domain/user package's GoogleVerifier interface, so the domain layer doesn't need to import an
// infrastructure package just to describe the shape of a verified Google identity.
type googleVerifierAdapter struct{ v *googleoauth.Verifier }

func (a googleVerifierAdapter) Verify(ctx context.Context, idToken string) (*userdomain.GoogleClaims, error) {
	claims, err := a.v.Verify(ctx, idToken)
	if err != nil {
		return nil, err
	}
	return &userdomain.GoogleClaims{Subject: claims.Subject, Email: claims.Email, EmailVerified: claims.EmailVerified, Name: claims.Name}, nil
}

type tripTransactor struct{ db *gorm.DB }

func (t tripTransactor) CreateTripWithPlanner(ctx context.Context, trip *domaintrip.Trip, part *domainparticipant.Participant) (*domaintrip.Trip, error) {
	var created *domaintrip.Trip
	err := t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = tripsdb.New(tx).Create(ctx, trip)
		if err != nil {
			return err
		}
		_, err = partsdb.New(tx).Create(ctx, part)
		return err
	})
	return created, err
}

// Ping godoc
// @Summary Check API connectivity
// @Description Returns a fully populated envelope proving the API composition root is wired.
// @Tags system
// @Produce json
// @Success 200 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /ping [get]
func (s *Service) Ping(c *gin.Context) {
	response.OK(c, "PONG", gin.H{"pong": true})
}
