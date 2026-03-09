package api

import (
	"encoding/json"
	"strings"
	"time"
)

// Pagination holds pagination information.
type Pagination struct {
	Page       int  `json:"page"`
	PerPage    int  `json:"per_page"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	TotalKnown bool `json:"-"`
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

// LoginRequest is sent to /auth/login.
type LoginRequest struct {
	Description string `json:"description"`
	ExpiresIn   int    `json:"expires_in"`
}

// Channel represents a content channel summary.
type Channel struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	EntryCount  *int      `json:"entry_count,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// ChannelSummary is the lightweight list/get projection for channels.
type ChannelSummary = Channel

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

// Page represents a page summary.
type Page struct {
	ID         string         `json:"id"`
	Fullpath   string         `json:"fullpath,omitempty"`
	Parent     string         `json:"parent,omitempty"`
	ParentPath string         `json:"parent_path,omitempty"`
	Slug       string         `json:"slug,omitempty"`
	Title      string         `json:"title,omitempty"`
	Template   string         `json:"template,omitempty"`
	Published  bool           `json:"published,omitempty"`
	Locale     string         `json:"locale,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at,omitempty"`
}

// PageSummary is the lightweight list projection for pages.
type PageSummary = Page

// Menu represents a navigation menu summary.
type Menu struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Handle    string     `json:"handle,omitempty"`
	Slug      string     `json:"slug,omitempty"`
	Items     []MenuItem `json:"items,omitempty"`
	CreatedAt time.Time  `json:"created_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at,omitempty"`
}

// MenuSummary is the lightweight list projection for menus.
type MenuSummary = Menu

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
	ID               string    `json:"id"`
	URL              string    `json:"url,omitempty"`
	Slug             string    `json:"slug,omitempty"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	Status           string    `json:"status,omitempty"`
	Price            float64   `json:"price,omitempty"`
	Currency         string    `json:"currency,omitempty"`
	SKU              string    `json:"sku,omitempty"`
	Inventory        int       `json:"inventory,omitempty"`
	Published        bool      `json:"published,omitempty"`
	CurrentStock     int       `json:"current_stock,omitempty"`
	Digital          bool      `json:"digital,omitempty"`
	RequiresShipping bool      `json:"requires_shipping,omitempty"`
	OnSale           bool      `json:"on_sale,omitempty"`
	OnSalePrice      float64   `json:"on_sale_price,omitempty"`
	VariantsEnabled  bool      `json:"variants_enabled,omitempty"`
	KeepStock        bool      `json:"keep_stock,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
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

// UnmarshalJSON accepts both modern and legacy customer field names.
func (c *Customer) UnmarshalJSON(data []byte) error {
	type customerAlias Customer
	var payload struct {
		customerAlias
		FirstNameAlt string `json:"firstname"`
		LastNameAlt  string `json:"lastname"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*c = Customer(payload.customerAlias)
	if c.FirstName == "" {
		c.FirstName = payload.FirstNameAlt
	}
	if c.LastName == "" {
		c.LastName = payload.LastNameAlt
	}

	return nil
}

// UnmarshalJSON maps menu slug to Handle fallback.
func (m *Menu) UnmarshalJSON(data []byte) error {
	type menuAlias Menu
	var payload menuAlias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*m = Menu(payload)
	if m.Handle == "" {
		m.Handle = m.Slug
	}

	return nil
}

// UnmarshalJSON normalizes order fields from API responses.
func (o *Order) UnmarshalJSON(data []byte) error {
	type orderAlias Order
	var payload struct {
		orderAlias
		State  string `json:"state"`
		Totals *struct {
			Total float64 `json:"total"`
		} `json:"totals"`
		Customer *struct {
			ID string `json:"id"`
		} `json:"customer"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*o = Order(payload.orderAlias)
	if o.Status == "" {
		o.Status = payload.State
	}
	if o.Total == 0 && payload.Totals != nil {
		o.Total = payload.Totals.Total
	}
	if o.CustomerID == "" && payload.Customer != nil {
		o.CustomerID = payload.Customer.ID
	}

	return nil
}

// UnmarshalJSON trims channel names and keeps missing entry_count as nil.
func (c *Channel) UnmarshalJSON(data []byte) error {
	type channelAlias Channel
	var payload channelAlias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*c = Channel(payload)
	c.Name = strings.TrimSpace(c.Name)

	return nil
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

// ThemeResource represents a theme layout/template/snippet/asset entry.
type ThemeResource struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Permalink   string    `json:"permalink"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Folder      string    `json:"folder"`
	PublicURL   string    `json:"public_url"`
	Code        string    `json:"code"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	ChangedLive bool      `json:"changed_live,omitempty"`
	// ChangedBy field is omitted intentionally, it is not needed for CLI output.
}

// Upload represents an uploaded file.
type Upload struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	Size      int64     `json:"size,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Webhook represents a webhook.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	TargetURL string    `json:"target_url,omitempty"`
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
	Slug      string    `json:"slug,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// UnmarshalJSON maps blog slug to Handle fallback.
func (b *Blog) UnmarshalJSON(data []byte) error {
	type blogAlias Blog
	var payload blogAlias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*b = Blog(payload)
	if b.Handle == "" {
		b.Handle = b.Slug
	}

	return nil
}

// BlogPost represents a blog post.
type BlogPost struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug,omitempty"`
	TextContent string    `json:"text_content,omitempty"`
	Status      string    `json:"status,omitempty"`
	Author      string    `json:"author,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// Account represents an account accessible for the current site context.
type Account struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url,omitempty"`
	Plan         string    `json:"plan,omitempty"`
	SKUCount     int       `json:"sku_count,omitempty"`
	SiteCount    int       `json:"site_count,omitempty"`
	StorageCount int       `json:"storage_count,omitempty"`
	UsersCount   int       `json:"users_count,omitempty"`
	Locale       string    `json:"locale,omitempty"`
	Owner        string    `json:"owner,omitempty"`
	Sites        []string  `json:"sites,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// CollectionImage represents an image attached to a collection.
type CollectionImage struct {
	ID          string `json:"id"`
	Position    int    `json:"position,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Size        int64  `json:"size,omitempty"`
	URL         string `json:"url,omitempty"`
}

// Collection represents a product collection.
type Collection struct {
	ID             string                    `json:"id"`
	URL            string                    `json:"url,omitempty"`
	Name           string                    `json:"name"`
	Slug           string                    `json:"slug,omitempty"`
	Fullpath       string                    `json:"fullpath,omitempty"`
	Description    string                    `json:"description,omitempty"`
	ProductCount   int                       `json:"product_count,omitempty"`
	Status         string                    `json:"status,omitempty"`
	Type           string                    `json:"type,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	SEOTitle       string                    `json:"seo_title,omitempty"`
	SEODescription string                    `json:"seo_description,omitempty"`
	SEOKeywords    string                    `json:"seo_keywords,omitempty"`
	Images         []CollectionImage         `json:"images,omitempty"`
	FeaturedImage  *CollectionImage          `json:"featured_image,omitempty"`
	Translations   map[string]map[string]any `json:"translations,omitempty"`
	CreatedAt      time.Time                 `json:"created_at,omitempty"`
	UpdatedAt      time.Time                 `json:"updated_at,omitempty"`
}

// Coupon represents a coupon.
type Coupon struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Description      string           `json:"description,omitempty"`
	Reason           string           `json:"reason,omitempty"`
	State            string           `json:"state,omitempty"`
	CouponType       string           `json:"coupon_type,omitempty"`
	CouponPercentage float64          `json:"coupon_percentage,omitempty"`
	CouponAmount     float64          `json:"coupon_amount,omitempty"`
	CustomerSpecific bool             `json:"customer_specific,omitempty"`
	Code             string           `json:"code,omitempty"`
	Lifespan         string           `json:"lifespan,omitempty"`
	LifespanAmount   int              `json:"lifespan_amount,omitempty"`
	LifespanTime     string           `json:"lifespan_time,omitempty"`
	Start            string           `json:"start,omitempty"`
	StartType        string           `json:"start_type,omitempty"`
	Constraints      string           `json:"constraints,omitempty"`
	Requirements     string           `json:"requirements,omitempty"`
	RequiredValue    float64          `json:"required_value,omitempty"`
	RequiredAmount   int              `json:"required_amount,omitempty"`
	CollectionIDs    []string         `json:"collection_ids,omitempty"`
	ProductTypeIDs   []string         `json:"product_type_ids,omitempty"`
	Customers        []map[string]any `json:"customers,omitempty"`
	Redemptions      []map[string]any `json:"redemptions,omitempty"`
	Referral         map[string]any   `json:"referral,omitempty"`
	Referrer         map[string]any   `json:"referrer,omitempty"`
	CreatedAt        time.Time        `json:"created_at,omitempty"`
	UpdatedAt        time.Time        `json:"updated_at,omitempty"`
}

// Notification represents a notification template.
type Notification struct {
	ID           string                    `json:"id"`
	URL          string                    `json:"url,omitempty"`
	Slug         string                    `json:"slug,omitempty"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	Subject      string                    `json:"subject,omitempty"`
	Text         string                    `json:"text,omitempty"`
	HTML         string                    `json:"html,omitempty"`
	HTMLEnabled  bool                      `json:"html_enabled,omitempty"`
	Translations map[string]map[string]any `json:"translations,omitempty"`
	CreatedAt    time.Time                 `json:"created_at,omitempty"`
	UpdatedAt    time.Time                 `json:"updated_at,omitempty"`
}

// Role represents a customer role.
type Role struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Customers   []string       `json:"customers,omitempty"`
	Children    []string       `json:"children,omitempty"`
	Parents     []string       `json:"parents,omitempty"`
	ACL         map[string]any `json:"_acl,omitempty"`
	Owner       string         `json:"_owner,omitempty"`
	CreatedAt   time.Time      `json:"created_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
}

// Redirect represents a redirect rule.
type Redirect struct {
	ID        string    `json:"id"`
	URL       string    `json:"url,omitempty"`
	Source    string    `json:"source,omitempty"`
	Target    string    `json:"target,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// AppFunction represents a cloud function inside an app.
type AppFunction struct {
	Name string `json:"name"`
	SHA  string `json:"sha,omitempty"`
}

// AppRoute represents a cloud route inside an app.
type AppRoute struct {
	Order       int            `json:"order,omitempty"`
	Verb        string         `json:"verb,omitempty"`
	Path        string         `json:"path,omitempty"`
	Constraints map[string]any `json:"constraints,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
	SHA         string         `json:"sha,omitempty"`
}

// AppCallback represents a callback inside an app.
type AppCallback struct {
	Event     string    `json:"event,omitempty"`
	Type      string    `json:"type,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	URL       string    `json:"url,omitempty"`
	SHA       string    `json:"sha,omitempty"`
}

// AppJob represents a job inside an app.
type AppJob struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Every     string    `json:"every,omitempty"`
	SHA       string    `json:"sha,omitempty"`
}

// AppSchedule represents a schedule inside an app.
type AppSchedule struct {
	Name      string         `json:"name"`
	Timing    string         `json:"timing,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
	Cron      string         `json:"cron,omitempty"`
	SHA       string         `json:"sha,omitempty"`
}

// App represents an OAuth app.
type App struct {
	Name        string        `json:"name"`
	URL         string        `json:"url,omitempty"`
	Key         string        `json:"key,omitempty"`
	Domain      string        `json:"domain,omitempty"`
	CallbackURL string        `json:"callback_url,omitempty"`
	SDKVersion  string        `json:"sdk_version,omitempty"`
	Functions   []AppFunction `json:"functions,omitempty"`
	Routes      []AppRoute    `json:"routes,omitempty"`
	Callbacks   []AppCallback `json:"callbacks,omitempty"`
	Jobs        []AppJob      `json:"jobs,omitempty"`
	Schedules   []AppSchedule `json:"schedules,omitempty"`
	CreatedAt   time.Time     `json:"created_at,omitempty"`
	UpdatedAt   time.Time     `json:"updated_at,omitempty"`
}

// AppCodeFile represents an app cloud code file.
type AppCodeFile struct {
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	Code      string    `json:"code,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// UnmarshalJSON normalizes product fields from current and legacy API responses.
func (p *Product) UnmarshalJSON(data []byte) error {
	type productAlias Product
	var payload struct {
		productAlias
		Published *bool `json:"published"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*p = Product(payload.productAlias)
	if p.CurrentStock == 0 && p.Inventory != 0 {
		p.CurrentStock = p.Inventory
	}
	if p.Status == "" && payload.Published != nil {
		switch {
		case *payload.Published:
			p.Status = "published"
		case !*payload.Published:
			p.Status = "draft"
		}
	}

	return nil
}

// UnmarshalJSON normalizes upload metadata from nested source payloads.
func (u *Upload) UnmarshalJSON(data []byte) error {
	type uploadAlias Upload
	var payload struct {
		uploadAlias
		Source *struct {
			Filename    string `json:"filename"`
			URL         string `json:"url"`
			ContentType string `json:"content_type"`
			Size        int64  `json:"size"`
		} `json:"source"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*u = Upload(payload.uploadAlias)
	if payload.Source != nil {
		if u.Name == "" {
			u.Name = payload.Source.Filename
		}
		if u.URL == "" {
			u.URL = payload.Source.URL
		}
		if u.MimeType == "" {
			u.MimeType = payload.Source.ContentType
		}
		if u.Size == 0 {
			u.Size = payload.Source.Size
		}
	}

	return nil
}

// UnmarshalJSON normalizes webhook target_url to URL for CLI output.
func (w *Webhook) UnmarshalJSON(data []byte) error {
	type webhookAlias Webhook
	var payload webhookAlias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*w = Webhook(payload)
	if w.URL == "" {
		w.URL = w.TargetURL
	}

	return nil
}

// UnmarshalJSON normalizes app schedule timing for older CLI fields.
func (s *AppSchedule) UnmarshalJSON(data []byte) error {
	type scheduleAlias AppSchedule
	var payload scheduleAlias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*s = AppSchedule(payload)
	if s.Cron == "" {
		s.Cron = s.Timing
	}

	return nil
}

// JobRunResult represents a scheduled job response.
type JobRunResult struct {
	JID string `json:"jid"`
}

// Translation represents a translation entry.
type Translation struct {
	Key    string            `json:"key"`
	Value  string            `json:"value"`
	Locale string            `json:"locale,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}
