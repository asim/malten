package command

func init() {
	Register(&Command{
		Name:        "pro",
		Description: "Malten Pro membership info",
		Usage:       "/pro",
		Handler: func(ctx *Context, args []string) (string, error) {
			return ProInfo(), nil
		},
	})
}

// ProInfo returns information about Pro membership
func ProInfo() string {
	return `⭐ Malten Pro

Free (what you have now):
• Context & directions
• Nearby places
• Prayer times & reminders
• Step counter (24h)
• Saved places (local)
• Push notifications

Pro - £2.99/month or £24.99/year:
• Cloud backup & sync
• Saved places on all devices
• Step history & stats
• Timeline sharing
• Priority support
• Offline maps (coming)

Family - £4.99/month:
• Pro for up to 5 people
• Share locations with family
• Safety check-ins

Coming soon. Join the waitlist at malten.ai/pro`
}
