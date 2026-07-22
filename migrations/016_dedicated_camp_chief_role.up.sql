UPDATE users
SET is_admin = FALSE,
    subcamp_id = NULL
WHERE is_camp_chief = TRUE;
