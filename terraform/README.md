# Ephemerain TF

This TF config defines the infrastrcture for ephemerain. 

Prerequisites:
* Enable Google Cloud Memorystore API
* Set `gcp-project` variable to project ID
* Create env var `GOOGLE_CREDENTIALS` with contents of service account JSON key (e. g. `cat ~/Downloads/hc-*.json | tr '\n' ' ' | pbcopy`). Service account must have Owner permissions.

To deploy:

    terraform workspace select <environment>
    terraform apply -auto-approve -var ephemerain-server-container="ghcr.io/mwudka/ephemerain:latest" 
