# Products, Orders, Customers, Collections & Coupons

Commerce commands for the Nimbu CLI. All examples assume `--json` output.

## Products

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`, `fields`, `config`.

Identifier: product ID or slug.

```bash
nimbu products list --all --json
nimbu products get sku-123 --json
nimbu products create name="Wine Box" price:=29.9 status=active --json
nimbu products update sku-123 price:=34.5 seo.title="Gift box" --json
nimbu products delete sku-123 --force
nimbu products count --json
```

### Displayed fields

List columns: `id`, `slug`, `name`, `sku`, `price`, `status`.
Get also shows: `description`, `currency`, `current_stock`, `digital`, `requires_shipping`, `on_sale`, `on_sale_price`, `created_at`, `updated_at`.

### products fields

Shows custom field schema for the product model. Read-only.

```bash
nimbu products fields --json
```

### products config

Manages product custom field definitions (customizations) across sites.

| Subcommand | Purpose | Requires |
|------------|---------|----------|
| `config copy --from A --to B` | Replicate custom field schema | `--force` or interactive confirm if target has existing fields |
| `config diff --from A --to B` | Compare custom field schemas | read-only |

### products copy

Copies all products from one site to another.

```bash
nimbu products copy --from staging --to production --allow-errors --json
```

Flags: `--from`, `--to` (required), `--from-host`, `--to-host`, `--allow-errors`.

Required scope: `read_products` (on source).

## Orders

**Orders have NO create or delete commands.** The API treats orders as externally created records.

Subcommands: `list`, `get`, `update`, `count` only.

Identifier: order ID or order number.

```bash
nimbu orders list --status paid --all --json
nimbu orders get ORD-12345 --json
nimbu orders update ORD-12345 --status shipped --json
nimbu orders count --status pending --json
```

### Key constraints

- `update` is primarily for status transitions. Use `--status <value>` or inline `status=shipped`.
- `update` also accepts `--file` and inline assignments for other writable fields (e.g., `note=Done`), but the main use case is status changes.
- `list` and `count` accept `--status` to filter by order status.
- No copy command exists for orders.

### Displayed fields

List columns: `id`, `number`, `status`, `total`, `currency`, `customer_id`.
Get also shows: `created_at`, `updated_at`.

If an order has no number, the first 8 chars of the ID are used as display number.

Required scope: `read_orders`.

## Customers

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`, `fields`, `config`.

Identifier: customer ID or email.

```bash
nimbu customers list --all --json
nimbu customers get user@example.com --json
nimbu customers create email=user@example.com first_name=Ana --json
nimbu customers update user@example.com phone="+32123456" --json
nimbu customers delete user@example.com --force
nimbu customers count --json
```

### Displayed fields

List columns: `id`, `email`, `first_name`, `last_name`.
Get also shows: `phone`, `created_at`, `updated_at`.

### customers fields

Shows custom field schema for the customer model. Read-only.

```bash
nimbu customers fields --json
```

### customers config

Same as products config but for customer custom field definitions.

| Subcommand | Purpose |
|------------|---------|
| `config copy --from A --to B` | Replicate customer custom field schema |
| `config diff --from A --to B` | Compare customer custom field schemas |

### customers copy

Copies customers between sites with upsert logic.

```bash
nimbu customers copy --from staging --to production --upsert email --json
```

| Flag | Default | Purpose |
|------|---------|---------|
| `--upsert` | `email` | Comma-separated fields to match existing customers |
| `--password-length` | `12` | Generated password length for new customers |
| `--query` | | Raw query string appended to source list |
| `--where` | | Where expression for source selection |
| `--per-page` | | Items per page during fetch |
| `--allow-errors` | `false` | Continue on validation errors |

Required scope: `read_customers` (on source).

### --copy-customers flag on other commands

`channels entries copy` and `sites copy` both accept `--copy-customers`. When set, the copy operation automatically replicates any customers referenced by owner or customer-type fields in channel entries. This avoids broken references on the target site.

```bash
nimbu channels entries copy --from src/blog --to dst/blog --copy-customers --json
nimbu sites copy --from staging --to production --copy-customers --json
```

## Collections

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`, `copy`.

Standard CRUD for product collections. No config/fields subcommands.

List columns: `id`, `slug`, `name`, `status`, `type`, `product_count`.

```bash
nimbu collections list --all --json
nimbu collections copy --from staging --to production --json
```

## Coupons

Subcommands: `list`, `get`, `create`, `update`, `delete`, `count`.

No `copy` command. Standard CRUD for discount coupons.

List columns: `id`, `code`, `name`, `state`, `coupon_type`, `coupon_percentage`, `coupon_amount`.

```bash
nimbu coupons list --all --json
nimbu coupons create code=SUMMER25 name="Summer Sale" coupon_type=percentage coupon_percentage:=25 --json
```
