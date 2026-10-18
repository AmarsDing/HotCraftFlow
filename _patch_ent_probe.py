from pathlib import Path
p = Path(r"E:\project\FrameWork\apps\server\app\admin\internal\data\ent_client.go")
c = p.read_text(encoding="utf-8")
start = c.find("\t\t// ent ")
# prefer the comment right after migrate create about expression indexes
marker = "failed creating schema resources"
pos = c.find(marker)
if pos < 0:
    raise SystemExit("anchor not found")
# find first // comment after the Fatalf line for schema resources
sub = c[pos:]
# after `l.Fatalf(...)\n\t\t}\n\n`
idx = sub.find("\t\t// ")
if idx < 0:
    raise SystemExit("comment not found")
abs_start = pos + idx
end = c.find("func NewLogEntClient", abs_start)
if end < 0:
    raise SystemExit("NewLogEntClient not found")
# walk back to include only inside GetMigrate block - keep closing of if migrate
# We replace from business comments to just before closing `}` of GetMigrate and return
# Find the closing of GetMigrate: the `}` immediately before blank line / return client
chunk = c[abs_start:end]
# The GetMigrate block should close with `\t}` before return client / end of NewEntClient
# Actually looking at structure: after business SQL comes `\t}` then maybe ensureWarehouse then `return client` then `}`
print("---CHUNK HEAD---")
print(chunk[:200])
print("---CHUNK TAIL---")
print(chunk[-400:])
print("CHUNK_LEN", len(chunk))
