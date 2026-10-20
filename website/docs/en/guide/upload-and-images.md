# Uploading and managing images

This page is for signed-in users. An administrator configures storage, policies and groups first; users choose an allowed policy and upload.

## Where to open it

After signing in, open Upload images (`/upload`) or My images (`/images`) from navigation. On a phone, open navigation first.

## Upload images

1. Open Upload and drop, choose or paste images.
2. Select an available policy and optionally an album.
3. Choose visibility. Images are private by default. Visibility controls gallery inclusion, not access through shared direct links.
4. Start the upload and check each result. Failed items can be retried individually.

Your group controls file size, formats, capacity and frequency. Policies control originals, WebP, thumbnails and optional responsive versions. Client-side size checks are advisory; the server still validates uploads.

## Copy and use links

Results support URL, Markdown, HTML and BBCode, with original, WebP and thumbnail versions. Available versions depend on the policy; a WebP-only policy has no separate original file.

Use direct links in blogs, forums and documents. The current public direct-link model is unsuitable for confidential files that require authentication for every image access; see [Image visibility and direct links](./privacy-and-trash.md#image-visibility-and-direct-links).

## Manage existing images

- My images manages your own images. Administrators have a site-wide image page.
- Click an image filename for properties and EXIF; on wide screens in the new layout, clicking its information area also opens the right panel. Click the thumbnail for a large preview.
- To copy an existing image link on a computer, right-click its card → Original / WebP / Thumbnail → Copy URL / Markdown / HTML / BBCode. Missing versions are unavailable. If a touch browser cannot open this menu, use a computer; you can also copy links from upload results immediately after uploading.
- Rename from that same context menu and save; only the display name changes, so links remain unchanged.
- Select multiple images to reveal the bulk action bar for album moves, visibility changes and deletion.
- [Managing albums](./albums-and-gallery.md) covers organization; [Search syntax](./search.md) covers filtering and shareable search URLs.
- Deletion moves images to the [recycle bin](./privacy-and-trash.md#recycle-bin), retained for 7 days by default. Open the Recycle bin tab at the top of My images to restore or permanently delete them.

## Other clients

See [Lsky API and PicGo](./lsky-api.md) for PicGo, Typora, uPic and API tokens, [Native API v2](./api-v2.md) for scripts, and [Visitors and public access](./public-access.md) for anonymous uploads.
