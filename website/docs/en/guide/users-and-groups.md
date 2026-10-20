# Users and groups

This page is for administrators. Groups control upload destinations, file limits and quotas, not image visibility.

## Where to open it

Sign in as an administrator and open Administration:

- User groups (`/admin/groups`) → New group or Edit → configure limits and policies → save.
- Users (`/admin/users`) → New user or Edit → assign the group and other properties → save.
- Ordinary users do not see these menus. After changing a group, check policy options and limits on Upload images as a member of that group.

## Setup order

1. Create and test storage, then create an upload policy in [Storage and policies](./storage-and-policies.md).
2. Bind allowed policies to a group, select a default policy, and set capacity, file size, formats and upload frequency.
3. Create accounts or change their groups in User management. Administrators can manage site-wide images; ordinary users manage their own.

Users need an available policy to upload. A default policy must belong to the group's allowed policies.

## Registration and guests

| User | Setting | Effect |
| --- | --- | --- |
| Newly registered account | Administration → Site settings → Sign-up & sign-in → Default group | Assigned at registration |
| Existing account | User management → Edit user | Changes that account's group |
| Anonymous uploader | Administration → Site settings → Guests & gallery → Guest group | Supplies guest upload limits and policies |

Registration is closed by default. Enabling it does not enable guest uploads or the gallery. See [Sign-in protection](./sign-in-protection.md) for email and CAPTCHA, and [Visitors and public access](./public-access.md) for anonymous visitors.

## Capacity and disabled users

Quota counts stored originals, WebP, cloud thumbnails and responsive files, each object once; local thumbnail caches are excluded. Recycling releases user quota, although retained objects still occupy storage. Restore must satisfy quota again.

Disabled users cannot continue using their accounts, and their images are hidden from the gallery. Shared direct image URLs still follow storage access rules. Use image deletion to invalidate source URLs; see [Privacy and recycle bin](./privacy-and-trash.md).
