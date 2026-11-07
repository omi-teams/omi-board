"""Regenerate and compare bytes, including an intentional spec-drift regression."""
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUTPUTS = [ROOT / name for name in (
    'server/internal/api/api.gen.go',
    'web/src/lib/api/schema.d.ts',
    'clients/python/omi_board_client/_generated.py',
)]


def generate():
    subprocess.run(['go', 'generate', './...'], cwd=ROOT / 'server', check=True)
    subprocess.run(['npm', 'run', 'gen'], cwd=ROOT / 'web', check=True)


def check():
    before = {p: p.read_bytes() if p.exists() else None for p in OUTPUTS}
    generate()
    changed = [str(p.relative_to(ROOT)) for p in OUTPUTS if p.read_bytes() != before[p]]
    if changed:
        print('Generated code drift: ' + ', '.join(changed), file=sys.stderr)
    return not changed


if __name__ == '__main__':
    if '--test-drift' in sys.argv:
        spec = ROOT / 'api/openapi.yaml'
        original = spec.read_bytes()
        snapshots = {p: p.read_bytes() for p in OUTPUTS}
        try:
            if not check() or not check():
                raise SystemExit('Generation is stale or not repeatable')
            spec.write_bytes(original.replace(b'Database available', b'Database available (drift test)'))
            if spec.read_bytes() == original or check():
                raise SystemExit('Spec drift was not detected')
            print('PASS: repeatable generation and intentional spec drift detection')
        finally:
            spec.write_bytes(original)
            for p, data in snapshots.items():
                p.write_bytes(data)
    else:
        raise SystemExit(0 if check() else 1)
