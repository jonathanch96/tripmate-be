-- D-9: index FK/lookup columns that had no coverage.
CREATE INDEX ix_settlements_from_user ON tripmate.settlements (from_user_id);
CREATE INDEX ix_settlements_to_user ON tripmate.settlements (to_user_id);
CREATE INDEX ix_settlements_approved_by ON tripmate.settlements (approved_by_user_id);
CREATE INDEX ix_settlements_created_by ON tripmate.settlements (created_by_user_id);
CREATE INDEX ix_expenses_created_by ON tripmate.expenses (created_by_user_id);
CREATE INDEX ix_expenses_approved_by ON tripmate.expenses (approved_by_user_id);
CREATE INDEX ix_trip_invitations_invited_by ON tripmate.trip_invitations (invited_by);
CREATE INDEX ix_receipts_uploaded_by ON tripmate.receipts (uploaded_by);
