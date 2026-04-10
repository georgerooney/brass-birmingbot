terraform {
  backend "gcs" {}
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

data "google_project" "project" {
  project_id = var.project_id
}

locals {
  env_content = fileexists("../.env.local") ? file("../.env.local") : ""
}

# Enable Compute Engine API
resource "google_project_service" "compute_api" {
  project = var.project_id
  service = "compute.googleapis.com"

  disable_on_destroy = false
}

# Enable Artifact Registry API
resource "google_project_service" "artifact_registry_api" {
  project = var.project_id
  service = "artifactregistry.googleapis.com"

  disable_on_destroy = false
}

# Enable Container Registry API
resource "google_project_service" "container_registry_api" {
  project = var.project_id
  service = "containerregistry.googleapis.com"

  disable_on_destroy = false
}

# Enable Cloud Build API
resource "google_project_service" "cloudbuild_api" {
  project = var.project_id
  service = "cloudbuild.googleapis.com"

  disable_on_destroy = false
}

# Service Account for the VM
resource "google_service_account" "vm_sa" {
  account_id   = "brass-rl-vm-sa"
  display_name = "Service Account for Brass RL VM"
}

# Artifact Registry Repository
resource "google_artifact_registry_repository" "brass_repo" {
  location      = var.region
  repository_id = "brass-rl"
  description   = "Docker repository for Brass RL"
  format        = "DOCKER"
  
  depends_on = [google_project_service.artifact_registry_api]
}

# Grant bucket access to the service account
resource "google_storage_bucket_iam_member" "bucket_admin" {
  bucket = var.bucket_name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.vm_sa.email}"
}

# Grant Artifact Registry Reader permission to the VM Service Account
resource "google_project_iam_member" "artifact_registry_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.vm_sa.email}"
}

# Grant Cloud Build access to the auto-created source bucket
resource "google_storage_bucket_iam_member" "cloudbuild_source_reader" {
  bucket = "${var.project_id}_cloudbuild"
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${data.google_project.project.number}-compute@developer.gserviceaccount.com"
}

# Grant Cloud Build service account permission to upload to Artifact Registry
resource "google_project_iam_member" "cloudbuild_artifact_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${data.google_project.project.number}-compute@developer.gserviceaccount.com"
}

# Grant Cloud Build service account permission to write logs
resource "google_project_iam_member" "cloudbuild_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${data.google_project.project.number}-compute@developer.gserviceaccount.com"
}

# VPC Network
resource "google_compute_network" "vpc_network" {
  name                    = "brass-rl-network"
  auto_create_subnetworks = false
}

# Subnet
resource "google_compute_subnetwork" "vpc_subnet" {
  name          = "brass-rl-subnet"
  ip_cidr_range = "10.0.1.0/24"
  region        = var.region
  network       = google_compute_network.vpc_network.id
  
  private_ip_google_access = true
}

# Firewall rule to allow IAP SSH
resource "google_compute_firewall" "allow_iap_ssh" {
  name    = "allow-iap-ssh"
  network = google_compute_network.vpc_network.id

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["35.235.240.0/20"]
}

# Cloud Router
resource "google_compute_router" "router" {
  name    = "brass-rl-router"
  region  = var.region
  network = google_compute_network.vpc_network.id
}

# Cloud NAT
resource "google_compute_router_nat" "nat" {
  name                               = "brass-rl-nat"
  router                             = google_compute_router.router.name
  region                             = google_compute_router.router.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# GCE Instance
resource "google_compute_instance" "brass_vm" {
  depends_on = [google_project_service.compute_api]

  name         = "brass-rl-training"
  machine_type = var.machine_type

  boot_disk {
    initialize_params {
      image = "projects/deeplearning-platform-release/global/images/family/common-cu128-ubuntu-2204-nvidia-570"
      size  = 100
    }
  }

  guest_accelerator {
    type  = var.gpu_type
    count = var.gpu_count
  }

  network_interface {
    network    = google_compute_network.vpc_network.id
    subnetwork = google_compute_subnetwork.vpc_subnet.id
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  service_account {
    email  = google_service_account.vm_sa.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    install-nvidia-driver = "true"
    startup-script        = <<-EOT
      #!/bin/bash
      # Create app dir
      mkdir -p /app
      
      # Install Docker
      apt-get update
      apt-get install -y docker.io
      systemctl start docker
      systemctl enable docker
      
      # Write .env.local from metadata
      echo "${base64encode(local.env_content)}" | base64 -d > /app/.env.local
      
      # Authenticate Docker to Artifact Registry
      gcloud auth configure-docker ${var.region}-docker.pkg.dev --quiet
      
      # Run the container
      # Mount .env.local into the container at the expected location (python/.env.local or root .env.local?)
      # We updated train.py to look in parent dir of train.py, which is root if running from python/!
      # Wait, train.py uses `Path(__file__).parent.parent / ".env.local"`.
      # If train.py is in `/app/python/train.py`, its parent is `/app/python/`.
      # Its parent.parent is `/app/`.
      # So it expects it at `/app/.env.local`!
      
      docker run --gpus all -v /app/.env.local:/app/.env.local ${var.region}-docker.pkg.dev/${var.project_id}/brass-rl/brass-rl:latest
      
      # Shut down the instance after training completes
      sudo poweroff
    EOT
  }

  scheduling {
    preemptible       = true
    automatic_restart = false
  }
}
