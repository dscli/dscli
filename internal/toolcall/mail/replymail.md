# replymail

Reply to an existing mail message.

Reply to a mail by its ID. The recipient is automatically set to
the original mail's sender. If subject is not provided, "Re: <original subject>"
is used.

Parameters: id (required integer) — mail ID to reply to; subject
(optional string) — reply subject; body (optional, but subject or
body required) — reply body.
