`POST /api/flags/{key}` refuses a body without `enabled` (400) instead of switching the flag off, and answers 500 rather than 404 when the database write fails.
