The Discord connector now recognises the unset-token placeholder in the shape the secret store actually seeds, so switching Discord on with no token set stays a clean no-op instead of an auth loop.
