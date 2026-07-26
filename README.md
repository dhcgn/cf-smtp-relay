# cf-smtp-relay
A tiny, self-hosted SMTP-to-Cloudflare-Email-API bridge, written in Go and shipped as a Docker image. Point your self-hosted apps (Immich, Vaultwarden, Gitea, Nextcloud, …) at it like any normal SMTP relay — under the hood it forwards every message to Cloudflare Email Service over HTTPS instead of talking to a real mail relay.
