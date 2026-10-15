import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("backup", Path(__file__).with_name("backup_manifest.py"))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)

class BackupTests(unittest.TestCase):
    def fixture(self, root):
        password = "a" * 64
        (root / ".env").write_text(f"PANEL_PORT=2053\nMIKAN_POSTGRES_PASSWORD={password}\nMIKAN_DATABASE_URL=postgres://mikan:{password}@localhost/mikan?host=/run/postgresql\n")
        (root / "compose.citrus.yaml").write_bytes((Path(__file__).parent.parent / "compose.citrus.yaml").read_bytes())
        (root / "db.dump").write_bytes(b"PGDMP-test")
        (root / "data").mkdir()
        (root / "data" / "certificate").write_text("test")
        backup.create(root)

    def archive(self, root, path, evil=None):
        with tarfile.open(path, "w:gz") as t:
            for p in root.iterdir():
                t.add(p, arcname=p.name)
            if evil:
                info = tarfile.TarInfo(evil)
                info.size = 1
                t.addfile(info, io.BytesIO(b"x"))

    def test_round_trip_and_changed_database(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)/"stage";root.mkdir();self.fixture(root)
            archive=Path(d)/"good.tar.gz";self.archive(root,archive);backup.verify(archive)
            (root/"db.dump").write_bytes(b"corrupted")
            bad=Path(d)/"bad.tar.gz";self.archive(root,bad)
            with self.assertRaises(ValueError):backup.verify(bad)

    def test_path_traversal(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)/"stage";root.mkdir();self.fixture(root)
            archive=Path(d)/"bad.tar.gz";self.archive(root,archive,"../outside")
            with self.assertRaises(ValueError):backup.verify(archive)

    def test_changed_compose_even_with_recomputed_hash(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)/"stage";root.mkdir();self.fixture(root)
            (root/"compose.citrus.yaml").write_text("services: evil")
            (root/"manifest.json").unlink();backup.create(root)
            archive=Path(d)/"bad.tar.gz";self.archive(root,archive)
            with self.assertRaises(ValueError):backup.verify(archive)

if __name__=="__main__":unittest.main()
