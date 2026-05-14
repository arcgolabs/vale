# Traefik Compatibility

Vale accepts native `vale.*` Docker labels and a Traefik-compatible HTTP label
subset. The goal is practical migration compatibility rather than full Traefik
feature parity.

## Routers

| Traefik label | Vale projection |
| --- | --- |
| `traefik.enable` | Container enable/disable. |
| `traefik.http.routers.<name>.rule` | `Host`, `Path`, `PathPrefix`, `Method`, `Header`, `Headers`. |
| `traefik.http.routers.<name>.entrypoints` | Vale entrypoint names. Unknown names fall back to the default entrypoint. |
| `traefik.http.routers.<name>.middlewares` | Route middleware chain, provider suffix stripped. |
| `traefik.http.routers.<name>.service` | Route service, provider suffix stripped. |

## Services

| Traefik label | Vale projection |
| --- | --- |
| `traefik.http.services.<name>.loadbalancer.server.port` | Endpoint port. |
| `traefik.http.services.<name>.loadbalancer.server.scheme` | Endpoint scheme. |

## Middlewares

| Traefik middleware | Vale projection |
| --- | --- |
| `addprefix.prefix` | `add_prefix`. |
| `stripprefix.prefixes` | `strip_prefixes`. |
| `replacepath.path` | `replace_path`. |
| `replacepathregex.regex/replacement` | `replace_path_regex`. |
| `redirectscheme.*` | `redirect_scheme`. |
| `redirectregex.*` | `redirect_regex`. |
| `chain.middlewares` | Middleware chain. |
| `headers.customrequestheaders.*` | Request header mutation. |
| `headers.customresponseheaders.*` | Response header mutation. |
| `headers.accesscontrol*` | CORS options. |
| `headers.framedeny/contenttypenosniff/browserxssfilter/stsseconds/referrerpolicy` | Security headers. |
| `buffering.maxrequestbodybytes` | Request body limit. |
| `basicauth.realm/users` | Basic auth with plain `user:password` values. |
| `forwardauth.address`, `trustforwardheader`, `authrequestheaders`, `authresponseheaders`, `forwardbody`, `maxbodysize`, `maxresponsebodysize`, `timeout` | Forward auth middleware. |
| `compress` / `compress.minresponsebodybytes` | Gzip compression. |
| `ipallowlist.sourcerange` / `ipwhitelist.sourcerange` | IP allow list. |
| `ratelimit.average/burst` | Token-bucket rate limit. |

Unsupported Traefik labels are ignored during projection. Unknown Vale
middleware types still fail compilation.
