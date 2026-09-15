ALTER TABLE tripmate.trips ALTER COLUMN setting_approval_settlements SET DEFAULT TRUE;
-- The per-trip values are not restored: which trips had multiple currencies or early settlement
-- switched off is no longer recorded anywhere once they have all been set to TRUE.
