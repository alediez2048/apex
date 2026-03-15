# Scenario Coverage Report

Maps each PRD-named test scenario to the test function(s) and demo assertion(s) that exercise it.

## PRD 20 Named Test Scenarios

| # | Scenario | Test Function | Demo Act | System Path Exercised |
|---|----------|--------------|----------|----------------------|
| 1 | Happy Path E2E | `TestIntegration_HappyPath` | Act 1 | POST deposit → Vendor PASS → Funding OK → Ledger post → FundsPosted |
| 2 | Vendor Blur Rejection | `TestIntegration_VendorBlur` | Act 2 | POST deposit → Vendor IQA_BLUR → Rejected (422) |
| 3 | Vendor Glare Rejection | `TestIntegration_VendorGlare` | Act 2 | POST deposit → Vendor IQA_GLARE → Rejected (422) |
| 4 | Vendor MICR Failure | `TestIntegration_VendorMICRFailure` | Act 3 | POST deposit → Vendor MICR_FAILURE → confidence 0.42 → Analyzing (flagged) |
| 5 | Vendor Duplicate | `TestIntegration_VendorDuplicate` | — | POST deposit → Vendor DUPLICATE → Rejected |
| 6 | Vendor Amount Mismatch | `TestIntegration_VendorAmountMismatch` | — | POST deposit → Vendor MISMATCH → OCR differs → Analyzing (flagged) |
| 7 | Over Deposit Limit | `TestIntegration_OverDepositLimit` | Act 3 | POST $6,000 → Funding OVER_LIMIT → Rejected (422) |
| 8 | Funding Duplicate Detection | `TestIntegration_FundingDuplicateDetection` | — | POST same MICR twice → FUNDING.DUPLICATE on second |
| 9 | Operator Approve | `TestIntegration_OperatorApprove` | Act 4 | Flag → GET queue → POST approve → FundsPosted |
| 10 | Operator Reject | `TestIntegration_OperatorReject` | — | Flag → POST reject → Rejected with reason |
| 11 | Return and Reversal | `TestIntegration_ReturnAndReversal` | Act 6 | FundsPosted → POST return → 4 ledger entries → Returned + $30 fee |
| 12 | Settlement File Contents | `TestIntegration_SettlementFileContents` | Act 5 | FundsPosted → POST batch → X9 JSON with file_header/cash_letters/file_control |
| 13 | Unauthenticated Request | `TestIntegration_UnauthenticatedRequest` | — | Missing Bearer token → 401 structured error |
| 14 | Ineligible Account | `TestIntegration_IneligibleAccount` | — | Ineligible investor → 403 FUNDING.INELIGIBLE |
| 15 | Ledger Invariant | `TestLedgerInvariant_DebitsEqualCredits` | — | Multiple deposits + return → SUM(debits) == SUM(credits) |
| 16 | No Double Batching | `TestSettlement_NoDoubleBatching` | Act 5 | Batch → second batch → "no deposits to batch" |
| 17 | Return on Completed | `TestReturnOnCompletedDeposit` | — | Completed deposit → POST return → Returned (late return) |
| 18 | Settlement Cutoff (before) | `TestSettlementDate_BeforeCutoff_SameDay` | — | 2 PM CT → settlement_date = same day |
| 19 | Settlement Cutoff (after) | `TestSettlementDate_AfterCutoff_NextBusinessDay` | — | 7 PM CT → settlement_date = next business day |
| 20 | Settlement Friday Rollover | `TestSettlementDate_Friday7PM_Monday` | — | Friday 7 PM CT → settlement_date = Monday |

## Additional Test Coverage (beyond PRD 20)

### Authentication & Error Handling (6 tests)
| Test | Path |
|------|------|
| `TestAuth_MissingToken` | No Authorization header → 401 |
| `TestAuth_InvalidToken` | Bad Bearer token → 401 |
| `TestAuth_ValidToken` | Valid token → 200 |
| `TestErrorFormat_HasCodeAndMessage` | Error JSON has code + message fields |
| `TestDeposits_GetNotFound` | GET unknown transfer_id → 404 |
| `TestDeposits_ListInvalidDate` | Bad date param → 400 |

### Deposit CRUD (5 tests)
| Test | Path |
|------|------|
| `TestDeposits_ListEmpty` | GET /deposits on empty DB → [] |
| `TestDeposits_ListWithFilters` | GET /deposits?status=&account= → filtered |
| `TestDeposits_GetByID` | GET /deposits/{id} → full transfer JSON |
| `TestDeposits_History` | GET /deposits/{id}/history → event trail |
| `TestDeposits_GetByID_JSONShape` | Response includes all required fields |

### Operator Queue (4 tests)
| Test | Path |
|------|------|
| `TestOperator_QueueList` | GET /operator/queue → flagged items |
| `TestOperator_Approve` | POST approve → FundsPosted |
| `TestOperator_Reject` | POST reject → Rejected |
| `TestOperator_RejectWrongState` | Reject non-Analyzing → 409 |

### Accounts (2 tests)
| Test | Path |
|------|------|
| `TestAccounts_Balance` | GET /accounts/{id}/balance → net balance |
| `TestAccounts_Ledger` | GET /accounts/{id}/ledger → entry list |

### Settlement API (5 tests)
| Test | Path |
|------|------|
| `TestSettlement_PostBatches_NoAuth_401` | Missing auth → 401 |
| `TestSettlement_PostBatches_NoDeposits_200` | No unbatched → "no deposits to batch" |
| `TestSettlement_PostBatches_CreatesBatch_NoDoubleBatch` | Create + verify no double-batch |
| `TestSettlement_GetBatch_404` | Unknown batch_id → 404 |
| `TestSettlement_GetBatch_200` | GET batch → X9 JSON |

### Returns API (5 tests)
| Test | Path |
|------|------|
| `TestReturns_FundsPosted_200` | Return on FundsPosted → 200 + 4 entries |
| `TestReturns_Completed_200` | Return on Completed → 200 (late return) |
| `TestReturns_InvalidState_409` | Return on Requested → 409 |
| `TestReturns_DoubleReturn_409` | Return twice → 409 on second |
| `TestReturns_NotFound_404` | Return unknown ID → 404 |
| `TestReturns_LedgerInvariant` | Post-return ledger balance check |

### Domain Layer (13 tests)
| Test | Path |
|------|------|
| `TestAmount_ToDollars` | 15000 → "$150.00" |
| `TestParseAmount` | "150.00" → 15000 |
| `TestParseAmount_FractionalCentsRejected` | "150.001" → error |
| `TestParseAmount_EdgeCases` | Zero, large amounts, negative |
| `TestParseAmount_Invalid` | Non-numeric → error |
| `TestTransition_Invalid` | Requested→Completed → error |
| `TestTransition_Valid` | All valid transitions succeed |
| `TestTransition_InvalidPairs` | All invalid pairs fail |
| `TestValidTransitions_Copy` | Returns copy, not reference |
| `TestTransferModel` | Struct field defaults |
| `TestReturnFeeCents` | Fee = 3000 |
| `TestErrorCodesDefined` | All domain error codes present |
| `TestNewDomainError` | Error construction |

### Vendor Stub (8 tests)
| Test | Path |
|------|------|
| `TestStub_PASS` | PASS-* → CleanPass, confidence 0.98, MICR data |
| `TestStub_BLUR` | BLUR-* → IQA blur |
| `TestStub_GLARE` | GLARE-* → IQA glare |
| `TestStub_MICR` | MICR-* → MICR failure, confidence 0.42 |
| `TestStub_DUP` | DUP-* → Duplicate detected |
| `TestStub_MISMATCH` | MISMATCH-* → Amount mismatch, OCR differs |
| `TestStub_HeaderOverride` | X-Vendor-Scenario overrides prefix |
| `TestEnsureStubImages` | Synthetic PNGs created |

### Funding Engine (9 tests)
| Test | Path |
|------|------|
| `TestEngine_MissingAPIKey_401Body` | No key → structured 401 |
| `TestEngine_InvalidAPIKey` | Bad key → ACCOUNT_NOT_FOUND |
| `TestEngine_IneligibleDaveWilson` | Ineligible → INELIGIBLE |
| `TestEngine_OverLimit6000` | $6,000 → OVER_LIMIT |
| `TestEngine_DuplicateWithin30Days` | Same MICR → DUPLICATE |
| `TestEngine_IRAGetsIndividual` | IRA → INDIVIDUAL |
| `TestEngine_OmnibusApex` | CORR-APEX → OMNI-APEX-001 |
| `TestEngine_UnderLimitOK` | Under limit → approved |
| `TestMemoryDuplicate_OldRecordNotDuplicate` | 31-day-old → not duplicate |

### Pipeline (5 tests)
| Test | Path |
|------|------|
| `TestPipeline_CleanPath_PASS` | Full pipeline → FundsPosted |
| `TestPipeline_BLUR_Rejected` | Pipeline halts at vendor → Rejected |
| `TestPipeline_MICR_Flagged` | Pipeline halts at risk → Analyzing |
| `TestPipeline_StepLogged` | Each step creates deposit_event |
| `TestRiskScore` | Confidence thresholds → LOW/CRITICAL |

### Ledger (5 tests)
| Test | Path |
|------|------|
| `TestPost_CreatesExactlyOneDebitAndOneCredit` | Balanced pair created |
| `TestPost_RejectsNonPositiveAmount` | Zero/negative → error |
| `TestBalance_CorrectAfterMultiplePosts` | Net balance calculation |
| `TestLedgerInvariant` | Global debits == credits |
| `TestPost_TransactionRollsBackOnFailure` | Failed post → no entries |

### Settlement Cutoff (4 tests)
| Test | Path |
|------|------|
| `TestSettlementDate_BeforeCutoff_SameDay` | Before 6:30 PM CT → today |
| `TestSettlementDate_AfterCutoff_NextBusinessDay` | After cutoff → next day |
| `TestSettlementDate_Friday7PM_Monday` | Friday after cutoff → Monday |
| `TestAfterCutoff` | AfterCutoff() bool check |

## Demo Script Assertions (24 total)

| Act | Assertion | System Path |
|-----|-----------|-------------|
| 1 | POST /deposits → 201 | Happy path deposit creation |
| 1 | status = FundsPosted | Auto-approved, ledger posted |
| 1 | GET /deposits/{id} → 200 | Transfer retrieval |
| 1 | status = FundsPosted | Persisted state |
| 1 | GET /accounts/balance → 200 | Balance query |
| 2 | BLUR → 422 | IQA blur rejection |
| 2 | GLARE → 422 | IQA glare rejection |
| 3 | $6,000 → 422 | Over-limit rejection |
| 3 | status = Rejected | Funding rule enforcement |
| 3 | MICR → 201 | MICR failure flagged |
| 3 | status = Analyzing | Risk-scored for review |
| 4 | GET /operator/queue → 200 | Queue retrieval |
| 4 | POST approve → 200 | Operator approval |
| 4 | status = FundsPosted | Post-approval ledger posting |
| 5 | POST /settlement/batches → 201 | Batch generation |
| 5 | GET batch → 200 | X9 file retrieval |
| 5 | GET batch items → 200 | Batch item listing |
| 5 | Second batch → 200 (empty) | No double-batching |
| 5 | message = "no deposits to batch" | Idempotent batching |
| 6 | POST /returns → 200 | Return processing |
| 6 | status = Returned | Terminal state transition |
| 6 | fee_cents = 3000 | $30 fee applied |
| 6 | GET history → 200 | Event trail retrieval |
| 6 | INVESTOR_NOTIFIED present | Notification event logged |

## Summary

- **92 total tests** across 7 packages
- **20 PRD-named scenarios** fully covered
- **24 demo assertions** across 6 acts
- **All tests passing**, all demo assertions passing
