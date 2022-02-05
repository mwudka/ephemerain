variable "gcp-project" {}

variable "instance-type" {
  default     = "e2-standard-2"
  description = "GCP instance type to create"
}

variable "region" {
  default = "us-central1"
}

variable "zone" {
  default = "us-central1-c"
}
