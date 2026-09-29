/* eslint-disable max-classes-per-file -- one small error hierarchy, meaningless apart */

/** Base for retryInstall errors thrown before the CR is touched: nothing was
 *  deleted, so the caller can roll its UI state back. */
export class RetryAbortedError extends Error {}

/** Thrown by retryInstall when reading the existing installation failed. */
export class RetryReadError extends RetryAbortedError {}

/** Thrown by retryInstall when the retry would re-create the installation at
 *  a different version while replaying config written against the old
 *  version's schema — a mismatch the controller may terminally reject after
 *  the original CR is already gone. */
export class RetryConfigVersionError extends RetryAbortedError {}
