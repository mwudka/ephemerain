terraform {
  required_providers {
    dns = {
      source  = "hashicorp/dns"
      version = "3.2.1"
    }
  }
}

variable "server" {}
variable "port" {}

provider "dns" {
  update {
    server        = var.server
    port          = var.port
    key_name      = "this-is-my-key."
    key_algorithm = "hmac-md5"
    key_secret    = "3VwZXJzZWNyZXQ=="
  }
}

resource "dns_a_record_set" "a_record" {
  zone = "something.example.com."
  name = "a"
  addresses = ["1.2.3.4"]
}
