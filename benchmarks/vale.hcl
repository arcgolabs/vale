entrypoint "web" {
  address = ":8080"
}

admin {
  address = ":19090"
}

observability {
  access_log = false
  metrics    = false
}

health {
  interval = "30s"
  timeout  = "2s"
}

service "upstream" {
  strategy = "round_robin"

  endpoint {
    url    = "http://upstream:8080"
    weight = 1
  }
}

route "bench" {
  entrypoint  = "web"
  service     = "upstream"
  path_prefix = "/"
}
