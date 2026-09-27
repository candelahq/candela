"use client";

import { useCallback, useState } from "react";
import { create, type DescMessage, type MessageInitShape } from "@bufbuild/protobuf";
import { createValidator } from "@bufbuild/protovalidate";
import {
  CreateUserRequestSchema,
  SetBudgetRequestSchema,
  CreateGrantRequestSchema,
} from "@/gen/candela/v1/user_service_pb";
import { ModelCatalogEntrySchema } from "@/gen/candela/types/model_catalog_pb";

export interface ValidationError {
  field: string;
  message: string;
}

/**
 * Singleton protovalidate validator instance at module scope.
 * Reusing a single validator instance avoids recompilation overhead on each validation invocation.
 */
export const defaultValidator = createValidator();

function extractFieldName(field: unknown): string {
  if (Array.isArray(field) && field.length > 0) {
    const last = field[field.length - 1];
    if (last && typeof last === "object" && "name" in last && typeof last.name === "string") {
      return last.name;
    }
  }
  const str = String(field || "");
  const match = str.match(/\.([a-zA-Z0-9_]+)$/);
  if (match) {
    return match[1];
  }
  return str || "unknown";
}

/**
 * Validates a protobuf message against its schema's buf/validate annotations.
 * Returns an array of field-level errors.
 */
export async function validateMessage<Desc extends DescMessage>(
  schema: Desc,
  values: MessageInitShape<Desc>,
): Promise<ValidationError[]> {
  try {
    const msg = create(schema, values);
    const result = defaultValidator.validate(schema, msg);

    if (result.kind === "valid") {
      return [];
    }

    if (result.kind === "error") {
      return [
        {
          field: "unknown",
          message: result.error?.message || "Validation failed",
        },
      ];
    }

    return (result.violations ?? []).map((v) => ({
      field: extractFieldName(v.field),
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

function findError(errors: ValidationError[], field: string): string | undefined {
  const snakeField = field.replace(/[A-Z]/g, (m) => `_${m.toLowerCase()}`);
  return errors.find((e) => e.field === field || e.field === snakeField)?.message;
}

/**
 * Hook for validating CreateUserRequest fields with protovalidate.
 */
export function useCreateUserValidation() {
  const [errors, setErrors] = useState<ValidationError[]>([]);

  const validate = useCallback(
    async (values: MessageInitShape<typeof CreateUserRequestSchema>) => {
      const errs = await validateMessage(CreateUserRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => findError(errors, field),
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
    async (values: MessageInitShape<typeof SetBudgetRequestSchema>) => {
      const errs = await validateMessage(SetBudgetRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => findError(errors, field),
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
    async (values: MessageInitShape<typeof CreateGrantRequestSchema>) => {
      const errs = await validateMessage(CreateGrantRequestSchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => findError(errors, field),
    [errors],
  );

  return { errors, validate, getError, clearErrors: () => setErrors([]) };
}

/**
 * Hook for validating a ModelCatalogEntry before upserting via UpdateModelCatalogEntry.
 * Validates the entry fields against the proto's buf/validate annotations.
 */
export function useCatalogEntryValidation() {
  const [errors, setErrors] = useState<ValidationError[]>([]);

  const validate = useCallback(
    async (values: MessageInitShape<typeof ModelCatalogEntrySchema>) => {
      const errs = await validateMessage(ModelCatalogEntrySchema, values);
      setErrors(errs);
      return errs.length === 0;
    },
    [],
  );

  const getError = useCallback(
    (field: string) => findError(errors, field),
    [errors],
  );

  return { errors, validate, getError, clearErrors: () => setErrors([]) };
}

