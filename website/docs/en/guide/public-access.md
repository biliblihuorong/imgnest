# Visitors and public access

Under Administration → Site settings → Guests & gallery, administrators decide what visitors can see and whether they can upload. **The gallery, guest uploads and registration have independent switches.** Enabling one does not enable the others.

## Where to configure and verify it

1. Sign in as an administrator → Administration → Site settings → Guests & gallery (`/admin/settings/access`).
2. Change Public gallery, Public albums only or Guest uploads; inspect the count and images in the visitor preview.
3. Click Save settings, wait for success, then check your site root `/` and `/gallery` in a signed-out window. Previewing does not save settings.

## What visitors see

For a signed-out visitor opening the site's root URL `/`:

| Gallery | Guest uploads | Result |
| --- | --- | --- |
| Off | Off | Redirect to `/login` |
| On | Off | Public home `/home`, the latest 12 public images and a full gallery link; uploading requires sign-in |
| Off | On | Public home with guest uploads and no gallery |
| On | On | Public home with both guest uploads and public images |

Guest uploads also require a valid guest group. Select one explicitly in settings; for legacy configurations, the server can fall back to a group marked as a guest group. With no valid group, guest uploads are treated as closed. Sign-up appears only when registration is enabled. Signed-in users opening `/` go to their console; `/home` offers an Open console link.

## The public gallery

The full gallery is `/gallery`; its data endpoint is `/api/gallery`. Neither requires sign-in. It lists active public images from all enabled users, newest first, **including existing public images**. Enabling the gallery never changes private images to public.

- Private images, recycled images and images belonging to disabled accounts are excluded.
- With Public albums only, both the image and its album must be public; unfiled images are excluded. Making an album public never changes the visibility of its private images.
- Cards show image names and uploader display names (or usernames), and open an image preview. The API never returns EXIF, GPS, email or upload IP.
- An empty result shows an empty state; a failed request shows an error and Retry.

### Preview before enabling

The Visitor preview in settings shows the total eligible image count for the selected album scope and can expand the latest 12 images. Administrators can use it while the gallery is off.

Changing Public albums only refreshes the preview. **Previewing does not save settings, enable the gallery or change image permissions.** Only the settings Save button applies switches. The count is a snapshot; later uploads or visibility changes can alter it.

### After closing

- Subsequent gallery requests return an empty list; `/gallery` says the gallery is closed.
- If guest uploads remain enabled, the public home keeps its upload area. With both off, new visitors go to sign-in.
- Images are not deleted and remain manageable by their owners and administrators.
- Shared original, WebP and cloud thumbnail URLs and album random links keep working. Open pages do not receive a live closure notification, and loaded images are not withdrawn.

See [Privacy and recycle bin](./privacy-and-trash.md#image-visibility-and-direct-links) for direct-link access and [Random image links](./random-links.md) to manage album sharing.

## Public home page and guest uploads

Enable guest uploads and select a guest group in Guests & gallery. Visitors can drop, choose or paste images and copy URL, Markdown, HTML or BBCode links without an account.

- File size, formats, capacity and policies come from the guest group; see [Users and groups](./users-and-groups.md). Limits are per visitor address and share the anonymous Lsky v1 counter.
- Guests cannot select albums. Their images are always private and excluded from the gallery.
- Guests cannot manage or delete their uploads later; an administrator can. People who need ongoing management should register and sign in.
- Signed-in users upload from their console instead of uploading as guests.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| The root URL shows sign-in | Whether the gallery and effective guest uploads are both off |
| The enabled gallery is empty | Image visibility, recycle status, disabled owners and the public-albums-only scope |
| Guest uploads are enabled but fail | Guest group, allowed policies, storage status and quotas |
| Image URLs work after closing the gallery | Direct links are independent; the switch only closes the public listing |
