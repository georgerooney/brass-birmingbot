output "instance_ip" {
  description = "The internal IP address of the GCE instance"
  value       = google_compute_instance.brass_vm.network_interface[0].network_ip
}

output "bucket_name" {
  description = "The name of the GCS bucket"
  value       = var.bucket_name
}
