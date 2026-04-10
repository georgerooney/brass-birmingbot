import os
from google.cloud import storage


def get_gcs_client():
    """Returns a GCS client.

    Will use credentials from environment or default service account.
    """
    return storage.Client()


def upload_file(local_path: str, bucket_name: str, gcs_path: str):
    """Uploads a single file to GCS."""
    try:
        client = get_gcs_client()
        bucket = client.bucket(bucket_name)
        blob = bucket.blob(gcs_path)
        blob.upload_from_filename(local_path)
        print(f"Uploaded {local_path} to gs://{bucket_name}/{gcs_path}")
    except Exception as e:
        print(f"Error uploading {local_path} to GCS: {e}")


def upload_directory(local_dir: str, bucket_name: str, gcs_prefix: str):
    """Uploads a directory recursively to GCS."""
    try:
        client = get_gcs_client()
        bucket = client.bucket(bucket_name)

        for root, dirs, files in os.walk(local_dir):
            for file in files:
                local_file_path = os.path.join(root, file)
                # Create relative path to maintain directory structure
                relative_path = os.path.relpath(local_file_path, local_dir)
                gcs_file_path = os.path.join(gcs_prefix, relative_path)

                blob = bucket.blob(gcs_file_path)
                blob.upload_from_filename(local_file_path)

        print(f"Uploaded directory {local_dir} to gs://{bucket_name}/{gcs_prefix}")
    except Exception as e:
        print(f"Error uploading directory {local_dir} to GCS: {e}")
