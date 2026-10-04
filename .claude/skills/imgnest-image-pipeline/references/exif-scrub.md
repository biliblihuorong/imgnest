# Lossless EXIF scrubbing (cloud originals)

Goal: remove location and identity data from the stored original without
touching compressed image data. The output must decode to exactly the same
pixels as the input.

## What to remove (`scrub_mode = gps`)

| Data | Where | Action |
| --- | --- | --- |
| GPS IFD | IFD0 tag `0x8825` (GPSInfo) → sub-IFD | Zero every GPS entry's value bytes (inline or at offset), then set the GPS IFD entry count to 0 |
| Artist | IFD0 `0x013B` | Zero the value bytes |
| CameraOwnerName | Exif IFD `0xA430` | Zero the value bytes |
| BodySerialNumber | Exif IFD `0xA431` | Zero the value bytes |
| LensSerialNumber | Exif IFD `0xA435` | Zero the value bytes |
| MakerNote | Exif IFD `0x927C` | Archive original locally, then zero/drop opaque payload; vendor owner/serial fields are not limited to standard tags |
| XMP packet | separate block (see below) | Remove the whole block |

Keep: Make, Model, lens model, exposure, date/time, Orientation, ICC profile.

`scrub_mode = all`: remove the EXIF block entirely except Orientation and the
ICC profile (simplest: drop the APP1 Exif segment and re-insert a minimal
TIFF with only Orientation; ICC lives in APP2 and is untouched).

Zeroing values in place keeps every offset valid, so the TIFF structure never
needs rebuilding. Keep the tag entries' type and count; only blank the data.

## JPEG

- Walk markers from `FFD8`. Stop at `FFDA` (SOS) — never touch scan data.
- EXIF: `APP1` (`FFE1`) whose payload starts with `"Exif\0\0"`; the TIFF header
  follows (`II*\0` little-endian or `MM\0*` big-endian). Parse IFD0, follow
  `0x8769` (Exif IFD) and `0x8825` (GPS IFD) pointers. Offsets are relative to
  the TIFF header start.
- XMP: `APP1` whose payload starts with `"http://ns.adobe.com/xap/1.0/\0"`.
  Extended XMP: `"http://ns.adobe.com/xmp/extension/\0"`. Remove these
  segments entirely (splice out marker + length + payload).
- Guard every offset/length against the segment bounds; malformed EXIF →
  return an error; the caller rejects enabled-scrub uploads instead of storing
  unmodified private metadata in the cloud.

## PNG

- EXIF lives in the `eXIf` chunk (raw TIFF, no `"Exif\0\0"` prefix). Apply the
  same TIFF edits, then recompute the chunk CRC-32 over type + data.
- XMP lives in an `iTXt` chunk with keyword `XML:com.adobe.xmp` — remove the
  whole chunk.

## WebP (RIFF)

- `EXIF` chunk holds TIFF data (some writers prefix `"Exif\0\0"`; handle both).
  Edit in place; chunk size unchanged.
- `XMP ` chunk: remove it, subtract its padded size from the RIFF size field,
  and clear the XMP bit (bit 2) in the `VP8X` flags byte.

## HEIC / AVIF

Not scrubbed in v1 (EXIF is an `iloc`-addressed item inside ISOBMFF). Policy
`heif_mode` decides: `webp_only` (default), `keep`, or `reject`.

## Verification (always)

1. libvips loads the scrubbed bytes.
2. Re-parse EXIF from the output: no GPS, no serials, Make/Model unchanged.
3. For JPEG: byte length change equals the removed XMP segment sizes only.
4. Test fixtures: iPhone HEIC→JPEG export, Android JPEG, Sony/Canon/Nikon
   JPEG, Lightroom export with XMP GPS, PNG screenshot with eXIf, WebP from
   cwebp with `-metadata all`.

M2 actual evidence uses synthetic metadata/native-generated codecs: GPS,
identity/MakerNote/XMP, multiple blocks, post-SOS JPEG metadata, PNG CRC, GIF
XMP, TIFF pixel aliases, missing terminators and ISOBMFF item extent bounds.
The camera exports above remain future fixtures and are not claimed tested.

Classic TIFF metadata is archived by reachable IFD/value ranges, with explicit
20MiB full-source-fallback only for opaque private layouts; enabled original
scrub rejects unsupported layouts. BigTIFF rejects. HEIC/AVIF capture supports
iinfv2/v3, iloc0/1/2 and data construction0/1; unsupported layouts reject.
Owner/admin raw is never embedded in ImageView or thumbnail responses.
