from __future__ import annotations

import subprocess


def ssh_cmd(
    host: str,
    port: str,
    user: str,
    password: str,
    remote: str,
    timeout: int = 120,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [
            "sshpass",
            "-p",
            password,
            "ssh",
            "-o",
            "StrictHostKeyChecking=no",
            "-o",
            "ConnectTimeout=30",
            "-p",
            port,
            f"{user}@{host}",
            remote,
        ],
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def ssh_out(
    host: str,
    port: str,
    user: str,
    password: str,
    remote: str,
    timeout: int = 120,
) -> str:
    result = ssh_cmd(host, port, user, password, remote, timeout)
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or f"ssh exit {result.returncode}")
    return result.stdout


def grep_remote_marker(
    host: str,
    port: str,
    user: str,
    password: str,
    log_glob: str,
    marker: str,
) -> bool:
    try:
        out = ssh_out(
            host,
            port,
            user,
            password,
            f"grep -l '{marker}' {log_glob} 2>/dev/null | head -1 || true",
            timeout=60,
        )
        return bool(out.strip())
    except RuntimeError:
        return False
