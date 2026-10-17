# Albums and gallery

Albums organize your own images; the gallery is the site's public window, showing public images to visitors in a masonry layout.

## Albums

- Create, rename and delete albums on the Albums page. Deleting an album never deletes images — they simply leave the album and return to "unfiled".
- Each album can have a cover. An album's detail page always shows that album's images; changing the search terms never widens the scope.
- Pick an album while uploading, or move existing images in bulk.
- "Unfiled" is a fixed view of images that belong to no album, written as `album:unfiled` in [search](./search) queries.

## Bulk actions

Select multiple images in the list to:

| Action | Effect |
| --- | --- |
| Move to album | Bulk move; images leave the source album |
| Set visibility | Bulk toggle public / private |
| Delete | Moves to the recycle bin (7 days by default, see [Privacy and recycle bin](./privacy-and-trash)) |

## The public gallery

The gallery is a no-login public page (`/api/gallery`) showing **public** images and their thumbnails in a masonry layout. It never returns EXIF.

Control it under Admin → Settings → Visitors and gallery:

- **Gallery switch**: on / off for the whole site. Direct links to images keep working when the gallery is off.
- **Public albums only**: when enabled, public images that are not in a public album stop appearing in the gallery.

Opening a [random image link](./random-links) for an album is another common way to share; the two do not affect each other.

## How visibility fits in

An image's public / private flag decides whether it appears in the gallery and via public direct links; albums themselves have no visibility. To expose "a whole album", either mark its images public, or use a random link — a random link serves every image in the album including private ones, and the UI warns about this when you enable it.
