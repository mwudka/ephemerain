terraform {
  cloud {
    organization = "mwudka-blueprints"

    workspaces {
      tags = ["ephemerain"]
    }
  }
}