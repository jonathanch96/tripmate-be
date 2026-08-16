ALTER TABLE tripmate.expenses DROP CONSTRAINT IF EXISTS expenses_charged_amount_requires_currency;
ALTER TABLE tripmate.expenses DROP COLUMN IF EXISTS charged_currency;
ALTER TABLE tripmate.expenses DROP COLUMN IF EXISTS charged_amount;
