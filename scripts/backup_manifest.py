"""Validate Citrus Gate backup paths and SHA-256 hashes before extraction."""
import hashlib
import json
from pathlib import Path, PurePosixPath
import sys
import tarfile

MAX_SIZE = 4 * 1024**3
TOP = {".env", "compose.citrus.yaml", "data", "db.dump", "manifest.json"}

def digest(file):
    h = hashlib.sha256()
    for chunk in iter(lambda: file.read(1024 * 1024), b""):
        h.update(chunk)
    return h.hexdigest()

def create(root):
    hashes = {}
    for p in root.rglob("*"):
        if p.is_symlink():
            raise ValueError("symlink in backup data")
        if p.is_file():
            with p.open("rb") as f:
                hashes[p.relative_to(root).as_posix()] = digest(f)
    (root / "manifest.json").write_text(json.dumps({"format": 1, "files": hashes}, indent=2), encoding="utf-8")

def verify(path):
    with tarfile.open(path, "r:gz") as t:
        seen = set()
        files = {}
        size = 0
        members = t.getmembers()
        if len(members) > 100000:
            raise ValueError("too many archive entries")
        for m in members:
            p = PurePosixPath(m.name)
            name = p.as_posix()
            if not p.parts or p.is_absolute() or ".." in p.parts or "\\" in m.name or ":" in m.name or p.parts[0] not in TOP:
                raise ValueError("unsafe archive path")
            if name in seen or not (m.isdir() or m.isfile()):
                raise ValueError("duplicate or unsafe archive entry")
            seen.add(name)
            if m.isfile():
                size += m.size
                if size > MAX_SIZE:
                    raise ValueError("archive is too large")
                if name == "manifest.json":
                    if m.size > 16 * 1024**2:
                        raise ValueError("manifest too large")
                    manifest = json.load(t.extractfile(m))
                else:
                    files[name] = digest(t.extractfile(m))
        if "manifest.json" not in seen or not {".env", "db.dump", "compose.citrus.yaml"}.issubset(files):
            raise ValueError("missing backup files")
        if manifest.get("format") != 1 or files != manifest.get("files"):
            raise ValueError("backup checksum mismatch")
        # Never execute a compose definition carried in an untrusted backup.
        with (Path(__file__).resolve().parent.parent / "compose.citrus.yaml").open("rb") as f:
            if files["compose.citrus.yaml"] != digest(f):
                raise ValueError("backup compose differs from the local trusted template")
        env = t.extractfile(".env").read().decode("ascii")
        values = dict(line.split("=", 1) for line in env.splitlines())
        if set(values) != {"PANEL_PORT", "MIKAN_POSTGRES_PASSWORD", "MIKAN_DATABASE_URL"}:
            raise ValueError("unexpected environment fields")
        password = values["MIKAN_POSTGRES_PASSWORD"]
        if len(password) != 64 or any(c not in "0123456789abcdef" for c in password):
            raise ValueError("invalid database credential")
        if not values["PANEL_PORT"].isdigit() or not 1 <= int(values["PANEL_PORT"]) <= 65535:
            raise ValueError("invalid panel port")
        if values["MIKAN_DATABASE_URL"] != f"postgres://mikan:{password}@localhost/mikan?host=/run/postgresql":
            raise ValueError("backup database must use the local socket")
        print("Backup paths and hashes verified.")

if __name__ == "__main__":
    command, value = sys.argv[1:]
    if command == "create":
        create(Path(value))
    elif command == "verify":
        verify(value)
    else:
        raise SystemExit("create DIRECTORY | verify ARCHIVE")
