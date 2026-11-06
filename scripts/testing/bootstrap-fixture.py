#!/usr/bin/env python3
"""One offline integration fixture: trusted payload and signed hostile archives."""
import base64, hashlib, io, json, pathlib, subprocess, sys, tarfile, tempfile
repo = pathlib.Path(__file__).resolve().parents[2]
version = '0.7.0-stage5-a2'
filename = f'arcvpn-{version}-linux-amd64.tar.gz'
real = repo / 'dist' / f'installer-{version}'
def run(args, **kw): return subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kw)
with tempfile.TemporaryDirectory() as tmp:
    root = pathlib.Path(tmp)
    key = root / 'key.pem'
    run(['openssl','genpkey','-algorithm','ED25519','-out',str(key)])
    public = run(['openssl','pkey','-in',str(key),'-pubout']).stdout.decode()
    original = (repo / 'deploy/bootstrap-install.sh').read_text()
    start = original.index('-----BEGIN PUBLIC KEY-----')
    end = original.index('-----END PUBLIC KEY-----', start) + len('-----END PUBLIC KEY-----')
    script = root / 'bootstrap.sh'
    script.write_text(original[:start] + public.strip() + original[end:])
    with tarfile.open(real / filename) as archive:
        originals = [(member, archive.extractfile(member).read()) for member in archive]
    for case in ('valid','traversal','symlink','hardlink','duplicate','setuid','checksum-path','tamper','bad-signature'):
        directory = root / case
        directory.mkdir()
        payload = directory / filename
        with tarfile.open(payload, 'w:gz', format=tarfile.USTAR_FORMAT) as archive:
            for old, data in originals:
                item = tarfile.TarInfo(old.name)
                item.mode = old.mode
                if case == 'setuid' and old.name == 'install.sh': item.mode |= 0o4000
                if case == 'checksum-path' and old.name == 'dist/SHA256SUMS':
                    data += b'0' * 64 + b'  ../../escape\n'
                item.size = len(data)
                archive.addfile(item, io.BytesIO(data))
            if case in ('traversal','symlink','hardlink','duplicate'):
                item = tarfile.TarInfo('../escape' if case == 'traversal' else 'install.sh')
                item.mode = 0o600
                if case in ('symlink','hardlink'):
                    item.type = tarfile.SYMTYPE if case == 'symlink' else tarfile.LNKTYPE
                    item.linkname = '/tmp/escape'
                archive.addfile(item, io.BytesIO())
        metadata = json.loads((real / (filename + '.manifest.json')).read_bytes())
        metadata['size'] = payload.stat().st_size
        metadata['sha256'] = hashlib.sha256(payload.read_bytes()).hexdigest()
        manifest = directory / (filename + '.manifest.json')
        manifest.write_text(json.dumps(metadata) + '\n')
        signed = root / 'signed'
        signed.write_bytes(b'ArcVPN installer manifest v1\n' + manifest.read_bytes())
        sig = directory / (filename + '.manifest.sig')
        run(['openssl','pkeyutl','-sign','-rawin','-inkey',str(key),'-in',str(signed),'-out',str(sig)])
        if case == 'tamper':
            with payload.open('ab') as out: out.write(b'x')
        if case == 'bad-signature': sig.write_bytes(bytes(64))
        result = subprocess.run(['bash',str(script),'--verify-only','--from-dir',str(directory)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if (result.returncode == 0) != (case == 'valid'):
            sys.exit(f'{case}: unexpected result\n{result.stdout.decode()}\n{result.stderr.decode()}')
        print(f'PASS {case}')
    result = subprocess.run(['bash',str(script),'--from-dir',str(root)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    assert result.returncode != 0 and b'requires --verify-only' in result.stderr
    assert not (root / 'escape').exists()
    print('PASS offline source cannot install; no path escape')
