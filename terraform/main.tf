provider "google" {
  project = var.gcp-project
  region  = var.region
  zone    = var.zone
}

data "google_compute_image" "ubuntu-2004-lts" {
  family  = "ubuntu-pro-2004-lts"
  project = "ubuntu-os-pro-cloud"
}

resource "google_compute_firewall" "default" {
  name    = "ephemerain-firewall"
  network = google_compute_network.network.self_link

  allow {
    protocol = "icmp"
  }

  allow {
    protocol = "tcp"
    ports    = ["22", "80", "443"]
  }

  allow {
    protocol = "udp"
    ports = ["53"]
  }


  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["dev"]
}


resource "google_compute_network" "network" {
  name                    = "ephemerain-network"
  auto_create_subnetworks = true
}


resource "google_compute_instance" "development" {
  name         = "ephemerain-development"
  machine_type = var.instance-type

  tags = ["dev"]

  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = data.google_compute_image.ubuntu-2004-lts.self_link
    }
  }

  network_interface {
    network = google_compute_network.network.self_link

    access_config {
    }
  }
}

locals {
  public-ip  = google_compute_instance.development.network_interface[0].access_config[0].nat_ip
  ssh-command = "gcloud compute ssh --zone '${var.zone}' '${google_compute_instance.development.name}'  --project '${var.gcp-project}'"
}
