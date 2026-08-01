# NovelCat CloudBeaver Entry

CloudBeaver can be listed in Peapod as an external infrastructure link. Keep the database console itself bound to the 111/test host loopback address, then expose it through the MinIO/edge host with a reverse SSH tunnel and Caddy.

## Peapod Link

Use the example config in `examples/external-links.novelcat.json` as `PEAPOD_LINKS_JSON`, or add the same row in Peapod Settings.

```bash
PEAPOD_LINKS_JSON='[{"id":"novelcat-prod-db-console","title":"NovelCat Prod DB (read-only)","url":"https://db.novelcat.cloud","description":"CloudBeaver read-only production DB console behind edge auth.","group":"Infrastructure"}]'
```

## Operator Access

```text
https://db.novelcat.cloud
```

Add a Tencent Cloud DNS `A` record first: `db.novelcat.cloud -> 159.75.153.53`.

The edge Caddy route should also require Basic Auth before proxying to CloudBeaver. Keep that password on the edge host, outside git.

## CloudBeaver Connection

Inside CloudBeaver, create a MySQL connection:

- Host: `prod-mysql-tunnel`
- Port: `13306`
- Database: `novel_factory`
- User: `novelcat_ro`

Keep the password in the server-side secret file and do not commit it.

## Stack Template

The compose template lives at `examples/novelcat-cloudbeaver.compose.yml`. It assumes:

- The 111 host keeps the private tunnel key in `./ssh/prod_db_tunnel_ed25519`.
- Production authorizes that key only for `172.22.0.8:3306`.
- CloudBeaver stays bound to `127.0.0.1:18978` on the 111 host.
- A separate systemd reverse tunnel forwards the 111 host loopback port to `127.0.0.1:18978` on the edge host.

The reverse tunnel systemd unit template lives at `examples/novelcat-cloudbeaver-edge-tunnel.service`, and the Caddy route template lives at `examples/novelcat-cloudbeaver.caddy`.
