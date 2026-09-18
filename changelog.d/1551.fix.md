Bound two call paths that could hang forever: the playout HTTP client now carries a request timeout, and every scheduled background job's tick runs under a deadline.
