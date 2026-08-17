DROP INDEX IF EXISTS tripmate.ix_settlements_trip_date;
ALTER TABLE tripmate.settlements DROP COLUMN settlement_date;
