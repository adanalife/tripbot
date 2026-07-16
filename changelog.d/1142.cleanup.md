Inject the tripbot config into `chatbot.App` (`New(cfg)` + an `App.Cfg` field) instead of reading the `c.Conf` package global — first step of retiring config-as-global.
