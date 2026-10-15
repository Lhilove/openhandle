-- 0002_payment_history.sql — index the recipient side of a payment.
--
-- History is now the union of the payments a user sent and the payments made to
-- them, so the recipient columns are queried on every history request. Only
-- transactions_recipient_address_idx existed; the handle link, which is how a
-- payment is attributed to the account that was paid, had no index.
CREATE INDEX IF NOT EXISTS transactions_recipient_handle_idx
    ON transactions (recipient_handle_id)
    WHERE recipient_handle_id IS NOT NULL;
