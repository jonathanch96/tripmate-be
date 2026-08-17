ALTER TABLE tripmate.settlements ADD COLUMN settlement_date DATE NOT NULL DEFAULT CURRENT_DATE;
CREATE INDEX ix_settlements_trip_date ON tripmate.settlements (trip_id, settlement_date DESC);
