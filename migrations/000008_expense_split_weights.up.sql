ALTER TABLE tripmate.expenses DROP CONSTRAINT expenses_split_type_check;
ALTER TABLE tripmate.expenses ADD CONSTRAINT expenses_split_type_check
    CHECK (split_type IN ('equal','manual','item','percent','shares'));

ALTER TABLE tripmate.expense_splits ADD COLUMN weight NUMERIC(20,6) CHECK (weight >= 0);
