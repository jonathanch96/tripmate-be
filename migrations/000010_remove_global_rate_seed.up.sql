-- Exchange rates are per-trip configuration set by each trip's owner, not a shared global
-- default. These four rows were seeded once as trip_id IS NULL rows and were being merged into
-- every trip's rate list, making it look like currency config was shared across trips.
DELETE FROM tripmate.exchange_rates WHERE trip_id IS NULL AND source = 'seed';
