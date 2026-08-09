package receipt

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/storage"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	expenseresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/expense"
	"github.com/shopspring/decimal"
)

type controller struct {
	trips                                tripdomain.Service
	parts                                participantdomain.Service
	service                              receiptdomain.Service
	uploadUser, extractUser, extractTrip *middleware.RateLimiter
}

func NewController(trips tripdomain.Service, parts participantdomain.Service, service receiptdomain.Service) Controller {
	return &controller{trips: trips, parts: parts, service: service,
		uploadUser: middleware.NewRateLimiter(30, time.Hour), extractUser: middleware.NewRateLimiter(10, time.Hour),
		extractTrip: middleware.NewRateLimiter(40, 24*time.Hour)}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	routes := group.Group("/trips/:code/receipts", middleware.RequireTripMember(c.trips, c.parts))
	userKey := func(ctx *gin.Context) string { return identity.MustFromContext(ctx.Request.Context()).UserID.String() }
	tripKey := func(ctx *gin.Context) string { return tripctx.MustFromContext(ctx.Request.Context()).Trip.ID.String() }
	routes.POST("", middleware.RateLimit(c.uploadUser, userKey), c.upload)
	routes.GET("", c.list)
	routes.GET("/:id", c.get)
	routes.DELETE("/:id", c.delete)
	routes.POST("/:id/extract", middleware.RateLimit(c.extractUser, userKey), middleware.RateLimit(c.extractTrip, tripKey), c.extract)
	routes.POST("/:id/items", c.addItem)
	routes.PATCH("/:id/items/:itemId", c.updateItem)
	routes.DELETE("/:id/items/:itemId", c.deleteItem)
	routes.PUT("/:id/assignments", c.assign)
	routes.GET("/:id/shares", c.shares)
	routes.POST("/:id/convert", c.convert)
}

func actor(ctx *gin.Context) identity.Identity {
	return identity.MustFromContext(ctx.Request.Context())
}
func tripContext(ctx *gin.Context) tripctx.TripContext {
	return tripctx.MustFromContext(ctx.Request.Context())
}

func receiptID(ctx *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("RECEIPT_NOT_FOUND"))
		return uuid.Nil, false
	}
	return id, true
}

// upload godoc
// @Summary Upload a receipt image
// @Tags receipts
// @Security BearerAuth
// @Accept multipart/form-data
// @Param code path string true "Trip code"
// @Param file formData file true "JPEG, PNG, or WebP receipt (max 10 MB)"
// @Success 201 {object} response.Envelope{data=receiptResponse}
// @Router /trips/{code}/receipts [post]
func (c *controller) upload(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, storage.MaxImageBytes+(1<<20))
	header, err := ctx.FormFile("file")
	if err != nil {
		response.Error(ctx, apperror.Newf("VALIDATION_FAILED", "a receipt image is required"))
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, storage.MaxImageBytes+1))
	if err != nil {
		response.Error(ctx, apperror.Wrap(err, "INTERNAL_ERROR"))
		return
	}
	entity, err := c.service.Upload(ctx, actor(ctx), tripContext(ctx), receiptdomain.UploadInput{Data: data, Filename: header.Filename, ContentType: header.Header.Get("Content-Type")})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "RECEIPT_UPLOADED", fromReceipt(*entity))
}

// list godoc
// @Summary List trip receipts
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=[]receiptResponse}
// @Router /trips/{code}/receipts [get]
func (c *controller) list(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(ctx.DefaultQuery("per_page", "20"))
	filter := receiptdomain.Filter{Page: page, PerPage: perPage}
	if raw := strings.TrimSpace(ctx.Query("status")); raw != "" {
		status := domainreceipt.Status(raw)
		switch status {
		case domainreceipt.StatusUploaded, domainreceipt.StatusExtracting, domainreceipt.StatusExtracted, domainreceipt.StatusFailed, domainreceipt.StatusConverted:
			filter.Status = &status
		default:
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
	}
	rows, total, err := c.service.ListByTrip(ctx, tripContext(ctx), filter)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	data := make([]receiptResponse, len(rows))
	for index, row := range rows {
		data[index] = fromReceipt(row)
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	response.List(ctx, "RECEIPTS_FETCHED", data, response.Pagination{Page: page, PerPage: perPage, TotalItems: total, TotalPages: totalPages})
}

// get godoc
// @Summary Get a receipt with items, assignments, and a short-lived image URL
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Success 200 {object} response.Envelope{data=receiptResponse}
// @Router /trips/{code}/receipts/{id} [get]
func (c *controller) get(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	entity, err := c.service.Get(ctx, tripContext(ctx), id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RECEIPT_FETCHED", fromReceipt(*entity))
}

// extract godoc
// @Summary Extract and persist receipt line items
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Success 200 {object} response.Envelope{data=receiptResponse}
// @Router /trips/{code}/receipts/{id}/extract [post]
func (c *controller) extract(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	entity, err := c.service.Extract(ctx, actor(ctx), tripContext(ctx), id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RECEIPT_EXTRACTED", fromReceipt(*entity))
}

func parseAmount(value *string, field string) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(*value))
	if err != nil {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: field, Rule: "decimal", Message: "must be a decimal amount"}})
	}
	return &parsed, nil
}

func (c *controller) itemBelongsToReceipt(ctx *gin.Context, itemID uuid.UUID) bool {
	receiptID, ok := receiptID(ctx)
	if !ok {
		return false
	}
	entity, err := c.service.Get(ctx, tripContext(ctx), receiptID)
	if err != nil {
		response.Error(ctx, err)
		return false
	}
	for _, item := range entity.Items {
		if item.ID == itemID {
			return true
		}
	}
	response.Error(ctx, apperror.New("RECEIPT_NOT_FOUND"))
	return false
}

// addItem godoc
// @Summary Add a receipt line item
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Param body body itemRequest true "Line item"
// @Success 201 {object} response.Envelope{data=itemResponse}
// @Router /trips/{code}/receipts/{id}/items [post]
func (c *controller) addItem(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	var req itemRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Name == nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	quantity, err := parseAmount(req.Quantity, "quantity")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	unit, err := parseAmount(req.UnitPrice, "unit_price")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	total, err := parseAmount(req.TotalPrice, "total_price")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	input := receiptdomain.AddItemInput{Name: *req.Name}
	if quantity != nil {
		input.Quantity = *quantity
	}
	if unit != nil {
		input.UnitPrice = *unit
	}
	if total != nil {
		input.TotalPrice = *total
	}
	item, err := c.service.AddItem(ctx, actor(ctx), tripContext(ctx), id, input)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "RECEIPT_ITEM_ADDED", fromItem(*item))
}

// updateItem godoc
// @Summary Correct a receipt line item
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Param itemId path string true "Item ID"
// @Param body body itemRequest true "Line item changes"
// @Success 200 {object} response.Envelope{data=itemResponse}
// @Router /trips/{code}/receipts/{id}/items/{itemId} [patch]
func (c *controller) updateItem(ctx *gin.Context) {
	itemID, err := uuid.Parse(ctx.Param("itemId"))
	if err != nil {
		response.Error(ctx, apperror.New("RECEIPT_NOT_FOUND"))
		return
	}
	if !c.itemBelongsToReceipt(ctx, itemID) {
		return
	}
	var req itemRequest
	if err = ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	quantity, err := parseAmount(req.Quantity, "quantity")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	unit, err := parseAmount(req.UnitPrice, "unit_price")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	total, err := parseAmount(req.TotalPrice, "total_price")
	if err != nil {
		response.Error(ctx, err)
		return
	}
	item, err := c.service.UpdateItem(ctx, actor(ctx), tripContext(ctx), itemID, receiptdomain.UpdateItemInput{Name: req.Name, Quantity: quantity, UnitPrice: unit, TotalPrice: total})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RECEIPT_ITEM_UPDATED", fromItem(*item))
}

// deleteItem godoc
// @Summary Delete a receipt line item
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Param itemId path string true "Item ID"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/receipts/{id}/items/{itemId} [delete]
func (c *controller) deleteItem(ctx *gin.Context) {
	itemID, err := uuid.Parse(ctx.Param("itemId"))
	if err != nil {
		response.Error(ctx, apperror.New("RECEIPT_NOT_FOUND"))
		return
	}
	if !c.itemBelongsToReceipt(ctx, itemID) {
		return
	}
	if err = c.service.DeleteItem(ctx, actor(ctx), tripContext(ctx), itemID); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "RECEIPT_ITEM_DELETED")
}

// assign godoc
// @Summary Replace all receipt item assignments
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Param body body []assignmentRequest true "Assignments"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/receipts/{id}/assignments [put]
func (c *controller) assign(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	var req []assignmentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	input := make([]receiptdomain.ItemAssignmentInput, len(req))
	for index, row := range req {
		itemID, err := uuid.Parse(row.ItemID)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input[index].ItemID = itemID
		input[index].UserIDs = make([]uuid.UUID, len(row.UserIDs))
		for userIndex, raw := range row.UserIDs {
			input[index].UserIDs[userIndex], err = uuid.Parse(raw)
			if err != nil {
				response.Error(ctx, apperror.New("VALIDATION_FAILED"))
				return
			}
		}
	}
	if err := c.service.SetAssignments(ctx, actor(ctx), tripContext(ctx), id, input); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "ASSIGNMENTS_SAVED")
}

// shares godoc
// @Summary Calculate receipt shares
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Success 200 {object} response.Envelope{data=shareResponse}
// @Router /trips/{code}/receipts/{id}/shares [get]
func (c *controller) shares(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	value, err := c.service.Shares(ctx, tripContext(ctx), id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "SHARES_FETCHED", fromShares(value))
}

// convert godoc
// @Summary Convert a receipt into one item-split expense
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Param body body convertRequest true "Expense details and payers"
// @Success 201 {object} response.Envelope{data=expenseresponse.Expense}
// @Router /trips/{code}/receipts/{id}/convert [post]
func (c *controller) convert(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	var req convertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	input := receiptdomain.ConvertInput{Description: req.Description, Payers: make([]receiptdomain.PayerInput, len(req.Payers))}
	if req.ExpenseDate != "" {
		date, err := time.Parse("2006-01-02", req.ExpenseDate)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.ExpenseDate = &date
	}
	for index, row := range req.Payers {
		userID, err := uuid.Parse(row.UserID)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		amount, err := decimal.NewFromString(row.Amount)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.Payers[index] = receiptdomain.PayerInput{UserID: userID, Amount: amount}
	}
	entity, err := c.service.ConvertToExpense(ctx, actor(ctx), tripContext(ctx), id, input)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "EXPENSE_CREATED", expenseresponse.FromDomain(*entity, tripContext(ctx), actor(ctx).UserID))
}

// delete godoc
// @Summary Delete a receipt without deleting its expense
// @Tags receipts
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Receipt ID"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/receipts/{id} [delete]
func (c *controller) delete(ctx *gin.Context) {
	id, ok := receiptID(ctx)
	if !ok {
		return
	}
	if err := c.service.Delete(ctx, actor(ctx), tripContext(ctx), id); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "RECEIPT_DELETED")
}
