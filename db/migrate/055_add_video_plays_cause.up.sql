/* Why the clip changed. 'natural': playout advanced in corpus order.
   'resume': tripbot started with the clip already on screen. 'external': a
   jump tripbot did not send (the console, another client, playout
   restarting). Otherwise the chat command that caused it: 'timewarp',
   'jump', 'find', 'skip', 'seek'. '' is unknown, which is every row from
   before this column. */
ALTER TABLE video_plays ADD COLUMN cause TEXT NOT NULL DEFAULT '';
