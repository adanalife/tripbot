-- chat_messages is how many chat messages arrived in the sampling window (one
-- ~61s tick) ending at this row — the per-minute aggregate that stands in for
-- storing a row per message. NULL means no counter was wired for that tick
-- (or the row predates this column); 0 means the counter was wired and chat
-- was silent. Every message counts, commands and bots' messages included: the
-- total is an aggregate no reader can attribute to senders, and the events
-- table's bot-exclusion rule applies to per-user computation, not totals.
ALTER TABLE viewer_samples ADD COLUMN chat_messages INTEGER;
