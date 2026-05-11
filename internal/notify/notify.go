// Package notify — Windows native toast notifications.
//
// git.sr.ht/~jackmordaunt/go-toast/v2 kutubxonasi ishlatilgan (Wails allaqachon shuni ishlatadi).
// AppID — toast'da ko'rinadigan dastur nomi. Windows Settings'da ham shu nom ko'rinadi.
package notify

import (
	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

// Notifier — toast bildirishnomalarini yuboradi.
type Notifier struct {
	AppID   string
	Enabled bool
}

// New — yangi notifier yaratadi.
func New(appID string) *Notifier {
	return &Notifier{AppID: appID, Enabled: true}
}

// Notify — oddiy toast yuboradi.
func (n *Notifier) Notify(title, message string) error {
	if !n.Enabled {
		return nil
	}
	t := toast.Notification{
		AppID:   n.AppID,
		Title:   title,
		Body:    message,
		Actions: []toast.Action{
			{Content: "Ochish", Arguments: "open"},
		},
	}
	return t.Push()
}

// NotifyError — xato bildirishnomasi (qizilroq fon)
func (n *Notifier) NotifyError(title, message string) error {
	if !n.Enabled {
		return nil
	}
	t := toast.Notification{
		AppID: n.AppID,
		Title: title,
		Body:  message,
	}
	return t.Push()
}

// SetEnabled — bildirishnomalarni yoqish/o'chirish.
func (n *Notifier) SetEnabled(enabled bool) {
	n.Enabled = enabled
}
