import subprocess
import time
import requests
from pathlib import Path

def is_server_running(url="http://localhost:8765"):
    """Check if the Brass engine server is responding to health checks."""
    try:
        # Use a short timeout so we don't hang if the server is down
        requests.get(f"{url}/health", timeout=0.5).raise_for_status()
        return True
    except Exception:
        return False

def ensure_server(root_path: Path):
    """
    Checks if the server is running. If not, starts it.
    
    Args:
        root_path: Path to the root of the brass project (where /server exists)
        
    Returns:
        The subprocess.Popen object if the server was started by this call,
        or None if it was already running.
    """
    if is_server_running():
        return None

    # Try pre-built binary first, fall back to go run
    binary = root_path / "server" / "brass_server.exe"
    if binary.exists():
        cmd = [str(binary)]
    else:
        print("Pre-built server not found — using `go run ./server` (slower startup).")
        print(f"  Build with:  go build -o server\\brass_server.exe ./server")
        cmd = ["go", "run", "./server"]

    proc = subprocess.Popen(
        cmd,
        cwd=str(root_path),
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )

    print("Waiting for engine server...", end="", flush=True)
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        # Check if process crashed immediately
        if proc.poll() is not None:
            out, _ = proc.communicate()
            raise RuntimeError(
                f"Server exited unexpectedly:\n{out.decode(errors='replace')}"
            )
        
        if is_server_running():
            print(" ready.")
            return proc
            
        print(".", end="", flush=True)
        time.sleep(0.5)

    # If we got here, it timed out
    proc.terminate()
    raise RuntimeError("Server failed to start within 30s.")
