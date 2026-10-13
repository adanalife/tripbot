/* The operator's own marks on a chatter: starred picks out friends and family
   so the console can make them stand out in its lists, and note is free text
   about who someone is. Both are set from the console and read back by it;
   nothing in the bot's behaviour depends on either. */
ALTER TABLE users
  ADD COLUMN starred BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN note TEXT NOT NULL DEFAULT '';
