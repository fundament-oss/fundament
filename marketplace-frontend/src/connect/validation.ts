import { Code, ConnectError } from '@connectrpc/connect';
import { ViolationsSchema } from '../generated/buf/validate/validate_pb';
import connectErrorMessage from './error';

/** Per-field validation messages keyed by their proto field path (e.g. "asset_tag"). */
export type FieldErrors = Record<string, string>;

export interface ParsedValidation {
  /** Field-level messages to show inline, keyed by proto field name. */
  fields: FieldErrors;
  /**
   * Form-level message for the error banner: unmapped protovalidate violations
   * (no field path) or any non-validation error (e.g. AlreadyExists). Null when
   * every violation mapped to a field.
   */
  message: string | null;
}

/**
 * Parses a Connect error into inline field errors plus a global banner message.
 *
 * For `InvalidArgument` responses it reads protovalidate `Violations`: each
 * violation with a field path becomes an inline error; violations without one
 * fall back to the banner. Any other error becomes a banner-only message.
 */
export default function parseValidationError(err: unknown): ParsedValidation {
  const ce = ConnectError.from(err);

  if (ce.code === Code.InvalidArgument) {
    const fields: FieldErrors = {};
    const unmapped: string[] = [];
    ce.findDetails(ViolationsSchema)
      .flatMap((violations) => violations.violations)
      .forEach((v) => {
        const field = v.field?.elements.map((e) => e.fieldName).join('.') ?? '';
        if (field) fields[field] = v.message;
        else unmapped.push(v.message);
      });
    if (Object.keys(fields).length > 0) {
      return { fields, message: unmapped.length > 0 ? unmapped.join('\n') : null };
    }
  }

  return { fields: {}, message: connectErrorMessage(err) };
}
