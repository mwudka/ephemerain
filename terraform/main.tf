provider "google" {
  project = var.gcp-project
  region  = var.region
  zone    = var.zone
}

resource "google_compute_firewall" "production" {
  name    = "ephemerain-prod-firewall"
  network = google_compute_network.network.self_link

  allow {
    protocol = "icmp"
  }

  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }

  allow {
    protocol = "udp"
    ports = ["53"]
  }


  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["prod"]
}


resource "google_compute_network" "network" {
  name                    = "ephemerain-network"
  auto_create_subnetworks = true
}
module "container-vm" {
  source  = "terraform-google-modules/container-vm/google"
  version = "3.0.0"
  
  container = {
    image=var.ephemerain-server-container
    env = [
      {
        name = "REDIS_ADDRESS"
        value = "${google_redis_instance.storage.host}:${google_redis_instance.storage.port}"
      },
      {
        name = "LOG_FORMAT"
        value = "json"
      }
    ]
  }
}

data "cloudinit_config" "cloudinit" {
  gzip          = false
  base64_encode = false

  // Systemd-resolved takes over port 53, because of course it does. This obviously
  // causes problems with the DNS server, which needs to run on 53. To get around this,
  // we just stop the systemd resolve service. DNS seems to still work on the instance,
  // somehow.
  // TODO: Figure out a more elegant way of avoiding the port conflict.
  part {
    content_type = "text/x-shellscript"
    filename     = "startup.sh"
    content      = <<EOT
#!/usr/bin/env bash

systemctl stop systemd-resolved.service
EOT
  }
}

module "public-ipaddress" {
  source  = "terraform-google-modules/address/google"
  version = "3.1.0"
  region = var.region
  project_id = var.gcp-project
  address_type = "EXTERNAL"
  names  = [ "external-facing-ip"]
}


resource "google_redis_instance" "storage" {
  name           = "ephemerain-storage"
  memory_size_gb = 1

  authorized_network = google_compute_network.network.self_link
  location_id = var.zone
}

resource "google_compute_instance" "production" {
  name         = "ephemerain-production"
  machine_type = var.instance-type

  tags = ["prod"]

  allow_stopping_for_update = true

  network_interface {
    network = google_compute_network.network.self_link

    access_config {
      nat_ip = module.public-ipaddress.addresses[0]
    }
  }
  boot_disk {
    initialize_params {
      image = module.container-vm.source_image
    }
  }

  metadata = {
    gce-container-declaration = module.container-vm.metadata_value
    user-data = data.cloudinit_config.cloudinit.rendered
    google-logging-enabled = true
  }

  labels = {
    container-vm = module.container-vm.vm_container_label
  }

  service_account {
    email = data.google_compute_default_service_account.default.email
    scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/logging.admin",
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

data "google_compute_default_service_account" "default" {
}

resource "google_project_iam_member" "log-writer" {
  project = var.gcp-project
  role = "roles/logging.logWriter"
  member = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}


resource "google_project_iam_member" "metric-writer" {
  project = var.gcp-project
  role = "roles/monitoring.metricWriter"
  member = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}

provider "aws" {
  region = "us-west-2"
}

data "aws_route53_zone" "ephemerain" {
  name         = "ephemerain.com."
}

data "aws_route53_zone" "ephemerain-api" {
  name         = "ephemerain-api.com."
}

resource "aws_route53_record" "nsN" {
  count = 4
  zone_id = data.aws_route53_zone.ephemerain-api.zone_id
  name = "ns${count.index+1}.ephemerain-api.com"
  type = "A"
  ttl = "60"
  records = [local.dns-server-ips[count.index % length(local.dns-server-ips)]]
}


// TODO: Remove this after updating the nameservers on ephemerain.com to point to nsN.ephemerain-api.com
resource "aws_route53_record" "nsN-legacy" {
  count = 4
  zone_id = data.aws_route53_zone.ephemerain.zone_id
  name = "ns${count.index+1}.ephemerain.com"
  type = "A"
  ttl = "60"
  records = [local.dns-server-ips[count.index % length(local.dns-server-ips)]]
}


resource "aws_route53_record" "root" {
  zone_id = data.aws_route53_zone.ephemerain-api.zone_id
  name = "ephemerain.com"
  type = "A"
  ttl = "60"
  records = local.dns-server-ips
}


resource "aws_route53_record" "root-api" {
  zone_id = data.aws_route53_zone.ephemerain.zone_id
  name = "ephemerain-api.com"
  type = "A"
  ttl = "60"
  records = local.dns-server-ips
}


// TODO: For this to work, we need to import the NS records from route53. Unfortunately,
// importing when using TFC is a huge pain because it can only be done locally. Maybe
// local-exec would work as a hack?
# resource "aws_route53_record" "dns-server-ns-record" {
#   zone_id = data.aws_route53_zone.ephemerain.zone_id
#   name = "ephemerain.com"
#   type = "NS"
#   records = aws_route53_record.nsN[*].name
#   ttl = "60"
# }

locals {
  ssh-command = "gcloud compute ssh --zone '${var.zone}' '${google_compute_instance.production.name}'  --project '${var.gcp-project}'"
  dns-server-ips = [local.production-ip] 
  production-ip = google_compute_instance.production.network_interface[0].access_config[0].nat_ip
}
