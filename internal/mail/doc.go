// Package mail implements inter-AI messaging.
//
// Architecture:
//
//   - internal/mail          — Core domain logic (this package)
//   - internal/toolcall/mail — LLM tool registration & argument parsing
//
// The mail system enables explicit communication between AI maintainers.
// Each AI is identified by a name_id (from ai_names table). Senders are
// determined from the current session via ainame.GetNameID(). Recipients
// are looked up by case-insensitive name_en or email.
//
// Data Model:
//
//	mail       — id, sender_name_id, recipient_name_id, subject, body, is_read, created_at
//	mail_fts   — FTS5 external content table over mail(subject, body)
//
// Handlers:
//
//	HandleSendMail     — Send a mail to another maintainer by name/email
//	HandleReadMail     — Read mails (list or single) for the current maintainer
//	HandleMailSearch   — FTS5 search across mails
//	HandleReplyMail    — Reply to an existing mail
//	HandleDeleteMail   — Delete a mail (recipient only)
//	HandleContacts     — List all known contacts from ai_names
package mail
