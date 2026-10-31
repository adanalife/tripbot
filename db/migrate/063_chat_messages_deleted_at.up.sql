/* deleted_at marks a line the platform removed (a moderator's delete, a
   timeout, a ban), stamped from chat.deleted. The row stays: the log keeps
   what was said and shows it struck, the way the live chat log does. */
ALTER TABLE chat_messages ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;
