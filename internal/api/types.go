package api

import "time"

// Pagination holds pagination information.
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// User represents the current user.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Admin     bool      `json:"admin,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Site represents a Nimbu site.
type Site struct {
	ID          string    `json:"id"`
	Subdomain   string    `json:"subdomain"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain,omitempty"`
	Description string    `json:"description,omitempty"`
	Locales     []string  `json:"locales,omitempty"`
	Timezone    string    `json:"timezone,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// AuthResponse is returned from login.
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Channel represents a content channel.
type Channel struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	EntryCount  int       `json:"entry_count,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// Entry represents a channel entry.
type Entry struct {
	ID        string         `json:"id"`
	Slug      string         `json:"slug,omitempty"`
	Title     string         `json:"title,omitempty"`
	Body      string         `json:"body,omitempty"`
	Position  int            `json:"position,omitempty"`
	Locale    string         `json:"locale,omitempty"`
	Published bool           `json:"published,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

// Page represents a page.
type Page struct {
	ID        string         `json:"id"`
	Slug      string         `json:"slug,omitempty"`
	Title     string         `json:"title,omitempty"`
	Template  string         `json:"template,omitempty"`
	Published bool           `json:"published,omitempty"`
	Locale    string         `json:"locale,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

// Menu represents a navigation menu.
type Menu struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Handle    string     `json:"handle,omitempty"`
	Items     []MenuItem `json:"items,omitempty"`
	CreatedAt time.Time  `json:"created_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at,omitempty"`
}

// MenuItem represents a menu item.
type MenuItem struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	URL      string     `json:"url,omitempty"`
	Target   string     `json:"target,omitempty"`
	Position int        `json:"position,omitempty"`
	Children []MenuItem `json:"children,omitempty"`
}

// Product represents a product.
type Product struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price,omitempty"`
	Currency    string    `json:"currency,omitempty"`
	SKU         string    `json:"sku,omitempty"`
	Inventory   int       `json:"inventory,omitempty"`
	Published   bool      `json:"published,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// Order represents an order.
type Order struct {
	ID         string    `json:"id"`
	Number     string    `json:"number,omitempty"`
	Status     string    `json:"status,omitempty"`
	Total      float64   `json:"total,omitempty"`
	Currency   string    `json:"currency,omitempty"`
	CustomerID string    `json:"customer_id,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// Customer represents a customer.
type Customer struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name,omitempty"`
	LastName  string    `json:"last_name,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Theme represents a theme.
type Theme struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ThemeFile represents a theme file.
type ThemeFile struct {
	Path      string    `json:"path"`
	Type      string    `json:"type,omitempty"`
	Size      int64     `json:"size,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Upload represents an uploaded file.
type Upload struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	Size      int64     `json:"size,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Webhook represents a webhook.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events,omitempty"`
	Active    bool      `json:"active,omitempty"`
	Secret    string    `json:"secret,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Token represents an API token.
type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Token     string    `json:"token,omitempty"`
	Scopes    []string  `json:"scopes,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Blog represents a blog.
type Blog struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Handle    string    `json:"handle,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// BlogPost represents a blog post.
type BlogPost struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug,omitempty"`
	Body      string    `json:"body,omitempty"`
	Published bool      `json:"published,omitempty"`
	Author    string    `json:"author,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Translation represents a translation entry.
type Translation struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Locale string `json:"locale,omitempty"`
}
