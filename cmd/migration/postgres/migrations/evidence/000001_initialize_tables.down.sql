DROP INDEX IF EXISTS payload_delivered_slot_idx;
DROP INDEX IF EXISTS payload_delivered_blockhash_idx;
DROP INDEX IF EXISTS payload_delivered_blocknumber_idx;
DROP INDEX IF EXISTS payload_delivered_proposerpubkey_idx;
DROP INDEX IF EXISTS payload_delivered_builderpubkey_idx;
DROP INDEX IF EXISTS payload_delivered_executionpayloadid_idx;
DROP INDEX IF EXISTS payload_delivered_value_idx;
DROP TABLE IF EXISTS payload_delivered;

DROP INDEX IF EXISTS builder_block_submission_slot_idx;
DROP INDEX IF EXISTS builder_block_submission_blockhash_idx;
DROP INDEX IF EXISTS builder_block_submission_blocknumber_idx;
DROP INDEX IF EXISTS builder_block_submission_builderpubkey_idx;
DROP INDEX IF EXISTS builder_block_submission_simsuccess_idx;
DROP INDEX IF EXISTS builder_block_submission_mostprofit_idx;
DROP INDEX IF EXISTS builder_block_submission_executionpayloadid_idx;
DROP TABLE IF EXISTS builder_block_submission;
 