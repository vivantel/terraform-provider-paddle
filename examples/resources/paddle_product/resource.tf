resource "paddle_product" "example" {
  name         = "Pro Plan"
  tax_category = "saas" # digital-goods, ebooks, implementation-services, professional-services, saas, software-programming-services, standard, training-services, website-hosting
  description  = "Full-featured plan for growing teams"
  # type        = "standard" # standard or custom — defaults to standard
  # image_url   = "https://example.com/logo.png" # must be a publicly accessible HTTPS URL
}
