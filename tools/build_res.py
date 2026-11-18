"""Builds MacroRec.ico and a COFF resource object (.syso) that Go's linker
embeds into the Windows exe as the application icon (group icon id 1)."""
import struct
import sys
from pathlib import Path

png_dir = Path(sys.argv[1])
out_dir = Path(sys.argv[2])
sizes = [16, 24, 32, 48, 64, 128, 256]
imgs = [(s, (png_dir / f"icon-{s}.png").read_bytes()) for s in sizes]
for s, data in imgs:
    assert data[:8] == b"\x89PNG\r\n\x1a\n", s

# ---------- .ico file ----------
ico = struct.pack("<HHH", 0, 1, len(imgs))
offset = 6 + 16 * len(imgs)
body = b""
for s, data in imgs:
    wh = 0 if s >= 256 else s
    ico += struct.pack("<BBBBHHII", wh, wh, 0, 0, 1, 32, len(data), offset + len(body))
    body += data
(out_dir / "MacroRec.ico").write_bytes(ico + body)

# ---------- resource tree (.rsrc section) ----------
RT_ICON, RT_GROUP_ICON, LANG = 3, 14, 0x0409
n = len(imgs)


def pad4(b):
    return b + b"\0" * (-len(b) % 4)


def directory(entries):
    """entries: list of (id, offset, is_subdir) sorted by id."""
    d = struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(entries))
    for ident, off, sub in entries:
        d += struct.pack("<II", ident, off | (0x80000000 if sub else 0))
    return d


# Sizes of the fixed-size parts, so offsets can be computed up front.
dir_size = lambda k: 16 + 8 * k
root_off = 0
icon_type_off = root_off + dir_size(2)
group_type_off = icon_type_off + dir_size(n)
lang_icon_off = [group_type_off + dir_size(1) + i * dir_size(1) for i in range(n)]
lang_group_off = group_type_off + dir_size(1) + n * dir_size(1)
data_entry_off = lang_group_off + dir_size(1)  # n icon entries, then the group entry
group_data_entry = data_entry_off + 16 * n
raw_off = group_data_entry + 16

group = struct.pack("<HHH", 0, 1, n)
for i, (s, data) in enumerate(imgs):
    wh = 0 if s >= 256 else s
    group += struct.pack("<BBBBHHIH", wh, wh, 0, 0, 1, 32, len(data), i + 1)

blobs = [pad4(d) for _, d in imgs] + [pad4(group)]
raw_offsets = []
cur = raw_off
for b in blobs:
    raw_offsets.append(cur)
    cur += len(b)

sec = b""
sec += directory([(RT_ICON, icon_type_off, True), (RT_GROUP_ICON, group_type_off, True)])
sec += directory([(i + 1, lang_icon_off[i], True) for i in range(n)])
sec += directory([(1, lang_group_off, True)])
for i in range(n):
    sec += directory([(LANG, data_entry_off + 16 * i, False)])
sec += directory([(LANG, group_data_entry, False)])

relocs = []  # section offsets that hold an RVA to fix up at link time
sizes_of = [len(d) for _, d in imgs] + [len(group)]
entry_pos = [data_entry_off + 16 * i for i in range(n)] + [group_data_entry]
for pos, raw, size in zip(entry_pos, raw_offsets, sizes_of):
    assert len(sec) == pos, (len(sec), pos)
    relocs.append(pos)  # OffsetToData; holds the section offset, linker adds the base RVA
    sec += struct.pack("<IIII", raw, size, 0, 0)
assert len(sec) == raw_off
for b in blobs:
    sec += b

# ---------- COFF object ----------
n_relocs = len(relocs)
sec_hdr_off = 20
raw_ptr = sec_hdr_off + 40
reloc_ptr = raw_ptr + len(sec)
sym_ptr = reloc_ptr + 10 * n_relocs

obj = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_ptr, 1, 0, 0)
obj += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(sec), raw_ptr, reloc_ptr, 0, n_relocs, 0, 0xC0000040)
obj += sec
for pos in relocs:
    obj += struct.pack("<IIH", pos, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB against symbol 0
obj += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # section symbol
obj += struct.pack("<I", 4)  # empty string table
(out_dir / "rsrc_windows_amd64.syso").write_bytes(obj)
print("wrote MacroRec.ico", len(ico + body), "bytes and rsrc_windows_amd64.syso", len(obj), "bytes")
