DELETE FROM feature_flags WHERE key = 'chatbot.gifts';
DELETE FROM feature_flags WHERE key = 'chatbot.find' AND platform IN ('tiktok', 'facebook', 'instagram');
