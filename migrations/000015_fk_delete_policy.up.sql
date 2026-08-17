-- D-6: every FK must declare an explicit ON DELETE behavior. These fell through to the implicit
-- Postgres default (NO ACTION). Users are never hard-deleted in normal operation (soft-delete via
-- deleted_at elsewhere in the schema), so RESTRICT is the safe explicit choice for money-bearing
-- rows: a user row can't vanish out from under an expense/settlement/receipt history.
ALTER TABLE tripmate.trips
    DROP CONSTRAINT trips_planner_id_fkey,
    ADD CONSTRAINT trips_planner_id_fkey FOREIGN KEY (planner_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.trip_participants
    DROP CONSTRAINT trip_participants_user_id_fkey,
    ADD CONSTRAINT trip_participants_user_id_fkey FOREIGN KEY (user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.trip_invitations
    DROP CONSTRAINT trip_invitations_invited_by_fkey,
    ADD CONSTRAINT trip_invitations_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.expenses
    DROP CONSTRAINT expenses_created_by_user_id_fkey,
    ADD CONSTRAINT expenses_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT,
    DROP CONSTRAINT expenses_approved_by_user_id_fkey,
    ADD CONSTRAINT expenses_approved_by_user_id_fkey FOREIGN KEY (approved_by_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT,
    DROP CONSTRAINT expenses_category_id_fkey,
    ADD CONSTRAINT expenses_category_id_fkey FOREIGN KEY (category_id) REFERENCES tripmate.expense_categories(id) ON DELETE SET NULL;

ALTER TABLE tripmate.expense_payers
    DROP CONSTRAINT expense_payers_user_id_fkey,
    ADD CONSTRAINT expense_payers_user_id_fkey FOREIGN KEY (user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.expense_splits
    DROP CONSTRAINT expense_splits_user_id_fkey,
    ADD CONSTRAINT expense_splits_user_id_fkey FOREIGN KEY (user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.settlements
    DROP CONSTRAINT settlements_from_user_id_fkey,
    ADD CONSTRAINT settlements_from_user_id_fkey FOREIGN KEY (from_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT,
    DROP CONSTRAINT settlements_to_user_id_fkey,
    ADD CONSTRAINT settlements_to_user_id_fkey FOREIGN KEY (to_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT,
    DROP CONSTRAINT settlements_approved_by_user_id_fkey,
    ADD CONSTRAINT settlements_approved_by_user_id_fkey FOREIGN KEY (approved_by_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT,
    DROP CONSTRAINT settlements_created_by_user_id_fkey,
    ADD CONSTRAINT settlements_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.receipts
    DROP CONSTRAINT receipts_uploaded_by_fkey,
    ADD CONSTRAINT receipts_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES tripmate.users(id) ON DELETE RESTRICT;

ALTER TABLE tripmate.receipt_item_assignments
    DROP CONSTRAINT receipt_item_assignments_user_id_fkey,
    ADD CONSTRAINT receipt_item_assignments_user_id_fkey FOREIGN KEY (user_id) REFERENCES tripmate.users(id) ON DELETE RESTRICT;
