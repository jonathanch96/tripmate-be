ALTER TABLE tripmate.expenses ADD COLUMN charged_amount NUMERIC(20,6) CHECK (charged_amount > 0);
ALTER TABLE tripmate.expenses ADD COLUMN charged_currency CHAR(3);
ALTER TABLE tripmate.expenses ADD CONSTRAINT expenses_charged_amount_requires_currency
    CHECK (charged_amount IS NULL OR charged_currency IS NOT NULL);
