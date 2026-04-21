"use client";

import { useCallback, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { createValidator } from "@bufbuild/protovalidate";
import {
  CreateUserRequestSchema,
  SetBudgetRequestSchema,
  CreateGrantRequestSchema,
} from "@/gen/candela/v1/user_service_pb";
import type { GenMessage } from "@bufbuild/protobuf/codegenv2";
import type { Message } from "@bufbuild/protobuf";

export interface ValidationError {
  field: string;
  message: string;
}

/**
 * Validates a protobuf message against its schema's buf/validate annotations.
 * Returns an array of field-level errors.
 */
async function validateMessage<T extends Message>(
  schema: GenMessage<T>,
  values: Record<string, unknown>,
): Promise<ValidationError[]> {
  try {
    const validator = createValidator();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const msg = create(schema, values as any);
    const result = validator.validate(schema, msg);

    if (result.kind === "valid") {
      return [];
    }

    return (result.violations ?? []).map((v) => ({
      field: v.field.toString() || "unknown",
      message: v.message || "Validation failed",
    }));
  } catch (err) {
    return [
      {
        field: "unknown",
        message: err instanceof Error ? err.message : "Validation failed",
      },
    ];
  }
}

/**
 * Hook for validating CreateUserRequest fields with protovalidate.
 */
export function useCreateUserValidation() {
  const [errors, setErrors] = useState<ValidationError[]>([]);

  const validate = useCallback(
    async (values: {
      email: string;
      displayName?: string;
      role?: number;
      dailyBudgetUsd?: number;
    }) => {
      const errs = await validateMessage(CreateUserRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => errors.find((e) => e.field === field)?.message,
    [errors],
  );

  return { errors, validate, getError, clearErrors: () => setErrors([]) };
}

/**
 * Hook for validating SetBudgetRequest fields.
 */
export function useSetBudgetValidation() {
  const [errors, setErrors] = useState<ValidationError[]>([]);

  const validate = useCallback(
    async (values: { userId: string; limitUsd: number; periodType?: number }) => {
      const errs = await validateMessage(SetBudgetRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => errors.find((e) => e.field === field)?.message,
    [errors],
  );

  return { errors, validate, getError, clearErrors: () => setErrors([]) };
}

/**
 * Hook for validating CreateGrantRequest fields.
 */
export function useCreateGrantValidation() {
  const [errors, setErrors] = useState<ValidationError[]>([]);

  const validate = useCallback(
    async (values: {
      userId: string;
      amountUsd: number;
      reason: string;
      startsAt?: unknown;
      expiresAt?: unknown;
    }) => {
      const errs = await validateMessage(CreateGrantRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => errors.find((e) => e.field === field)?.message,
    [errors],
  );

  return { errors, validate, getError, clearErrors: () => setErrors([]) };
}
