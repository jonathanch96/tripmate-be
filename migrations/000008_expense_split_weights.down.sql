ALTER TABLE tripmate.expense_splits DROP COLUMN IF EXISTS weight;

ALTER TABLE tripmate.expenses DROP CONSTRAINT expenses_split_type_check;
ALTER TABLE tripmate.expenses ADD CONSTRAINT expenses_split_type_check
    CHECK (split_type IN ('equal','manual','item'));
