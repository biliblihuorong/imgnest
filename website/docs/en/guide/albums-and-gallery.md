# Managing albums

Albums organize your own images. This page covers album operations; site-wide public access is configured separately.

## Albums

Sign in and open My albums (`/albums` on your site). Click an album’s **cover or name** to enter it. Edit on the card changes album properties; it does not open the detail page.

### Create an album and add images

1. Click New album, enter a name and optional description, visibility and cover, then click Create.
2. For new images, open Upload images and select this album before uploading.
3. For existing images, open My images, select images, choose the target album in the bulk action bar and click Move to album.
4. Return to My albums and click its cover or name to check the contents.

Use Edit on the card to change its name, description, cover or visibility, then Save. Use Delete and confirm to remove the album; its images are kept and become unfiled.

The Random image link button in the album detail header opens sharing controls. Follow [Random image links](./random-links.md#where-to-open-it) for the complete steps.

- Each album can have a cover. An album's detail page always shows that album's images; changing the search terms never widens the scope.
- Pick an album while uploading, or move existing images in bulk.
- "Unfiled" is a fixed view of images that belong to no album, written as `album:unfiled` in [search](./search.md) queries.

## Bulk actions

Select multiple images in the list to:

| Action | Effect |
| --- | --- |
| Move to album | Bulk move; images leave the source album |
| Set visibility | Bulk toggle public / private |
| Delete | Moves to the recycle bin (7 days by default, see [Privacy and recycle bin](./privacy-and-trash.md)) |

<a id="how-visibility-fits-in"></a>

## Album visibility

Albums and images each have a public/private flag. Making an album public does not publish its private images. Album visibility affects gallery inclusion only when the administrator enables Public albums only; see [Visitors and public access](./public-access.md#the-public-gallery).

An album's detail page in the console still requires sign-in. A public album flag does not create a public album-detail URL. Use a [random image link](./random-links.md) to share a random image from an album; it can include private images.

## The public gallery

Gallery switches, inclusion rules, previews and closure behavior are documented in [Visitors and public access](./public-access.md#the-public-gallery).

## Public home page and guest uploads

See the [visitor behavior table and guest upload instructions](./public-access.md). Image visibility and direct-link access are explained in [Privacy and recycle bin](./privacy-and-trash.md#image-visibility-and-direct-links).
