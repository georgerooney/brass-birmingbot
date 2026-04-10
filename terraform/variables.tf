variable "project_id" {
  description = "The GCP project ID"
  type        = string
}

variable "region" {
  description = "The region to deploy to"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "The zone to deploy to"
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "The machine type for the GCE instance"
  type        = string
  default     = "n1-standard-4" # N1 is good for attaching T4 GPUs
}

variable "gpu_type" {
  description = "The type of GPU to attach"
  type        = string
  default     = "nvidia-tesla-t4"
}

variable "gpu_count" {
  description = "The number of GPUs to attach"
  type        = number
  default     = 1
}

variable "bucket_name" {
  description = "The name of the GCS bucket for logs and weights"
  type        = string
}
