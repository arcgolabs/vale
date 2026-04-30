entrypoint "web" {
  address = ":8080"
}

proxy_engine = "stdlib"

admin {
  address = ":19090"
}

observability {
  access_log = true
  metrics    = true
}

health {
  interval = "5s"
  timeout  = "2s"
}

service "echo" {
  strategy = "round_robin"

  endpoint {
    url    = "http://127.0.0.1:8081"
    weight = 1
  }
}

route "echo-route" {
  entrypoint  = "web"
  service     = "echo"
  path_prefix = "/"
}
