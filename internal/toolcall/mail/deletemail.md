# deletemail

Delete a mail message by ID for the current maintainer.

Only the recipient can delete their own mails. Deletion removes
the mail from both the mail table and the FTS index.

Parameters: id (required integer) — mail ID to delete.
