INSERT INTO feature_flags (key, description, enabled, target_removal_date)
VALUES (
    'chatbot.weather',
    'Gates the !weather chat command (historical conditions at the dashcam location). Enable once the Open-Meteo lookup is verified in chat.',
    FALSE,
    DATE '2026-12-02'
)
ON CONFLICT (key) DO NOTHING;
