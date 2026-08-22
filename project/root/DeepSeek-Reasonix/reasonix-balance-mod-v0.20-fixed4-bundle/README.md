# Reasonix Balance Mod v0.20 Fixed4

Fixed4 closes the concrete Fixed3 blocker: the real DeepSeek provider config did not include `price`, while v0.16 strict pre-call admission intentionally fails closed when model pricing is unknown. Fixed4 supplies explicit conservative peak pricing before Provider.Stream and verifies it in the online gate.

The KZT ledger uses worst-case peak rates for safety. Reconciliation verifies that conservative ledger separately from the current time-band DeepSeek rate-card estimate.
