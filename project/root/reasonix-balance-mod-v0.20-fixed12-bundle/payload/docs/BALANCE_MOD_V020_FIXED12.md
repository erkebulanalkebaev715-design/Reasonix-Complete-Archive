# Balance Mod v0.20 Fixed12

Fixed12 fixes the final reconciliation blocker. The real gate now captures the exact provider Usage record at Agent.emitTurnUsage into a temporary 0600 receipt only when BALANCE_V20_USAGE_RECEIPT_PATH is explicitly set. The reconciler requires exact/non-estimated usage, Flash model identity, exactly one request, positive prompt/output tokens, cache-aware pricing, and agreement with the KZT ledger. Outside the real gate the receipt tap is a no-op. The installer makes no provider/network call.


Fixed12 installer compatibility hardening:
- supports the original/legacy Reasonix emitTurnUsage void form that emits an event inline;
- supports newer emitTurnUsage forms that return a CostQuote;
- inserts the exact usage receipt hook structurally inside emitTurnUsage rather than matching a formatting-specific sink/return block;
- installer remains provider-offline.
