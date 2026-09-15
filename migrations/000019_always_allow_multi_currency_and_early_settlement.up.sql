-- Holding several currencies and settling up before the trip ends are no longer per-trip choices:
-- every trip allows both and the toggles are gone from the UI. A trip that had either switched
-- off would otherwise stay restricted with no control left to turn it back on.
UPDATE tripmate.trips SET setting_multi_currency_enabled = TRUE WHERE setting_multi_currency_enabled = FALSE;
UPDATE tripmate.trips SET setting_allow_settlement_before_end = TRUE WHERE setting_allow_settlement_before_end = FALSE;

-- A new trip now starts with no approval gate at all; settlement approval is opted into per trip.
ALTER TABLE tripmate.trips ALTER COLUMN setting_approval_settlements SET DEFAULT FALSE;
