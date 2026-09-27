import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  defaultValidator,
  validateMessage,
  useCreateUserValidation,
  useSetBudgetValidation,
  useCreateGrantValidation,
  useCatalogEntryValidation,
} from "@/hooks/useProtoValidation";
import { CreateUserRequestSchema, SetBudgetRequestSchema } from "@/gen/candela/v1/user_service_pb";

describe("useProtoValidation", () => {
  describe("defaultValidator singleton", () => {
    it("is defined and reused as a module singleton", () => {
      expect(defaultValidator).toBeDefined();
      expect(typeof defaultValidator.validate).toBe("function");
    });

    it("validates messages using validateMessage directly", async () => {
      const validErrors = await validateMessage(CreateUserRequestSchema, {
        email: "alice@candela.dev",
        displayName: "Alice",
      });
      expect(validErrors).toEqual([]);

      const invalidErrors = await validateMessage(CreateUserRequestSchema, {
        email: "not-an-email",
      });
      expect(invalidErrors.length).toBeGreaterThan(0);
      expect(invalidErrors[0].field).toContain("email");
    });
  });

  describe("useCreateUserValidation", () => {
    it("passes validation for valid user details", async () => {
      const { result } = renderHook(() => useCreateUserValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          email: "user@candela.run",
          displayName: "Valid User",
          dailyBudgetUsd: 25.5,
        });
      });

      expect(isValid).toBe(true);
      expect(result.current.errors).toEqual([]);
      expect(result.current.getError("email")).toBeUndefined();
    });

    it("fails validation for invalid email", async () => {
      const { result } = renderHook(() => useCreateUserValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          email: "invalid-email-address",
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.errors.length).toBeGreaterThan(0);
      expect(result.current.getError("email")).toBeDefined();
    });

    it("fails validation for negative dailyBudgetUsd", async () => {
      const { result } = renderHook(() => useCreateUserValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          email: "valid@candela.run",
          dailyBudgetUsd: -10,
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.getError("daily_budget_usd")).toBeDefined();
      expect(result.current.getError("dailyBudgetUsd")).toBeDefined();
    });

    it("clears errors when clearErrors is called", async () => {
      const { result } = renderHook(() => useCreateUserValidation());

      await act(async () => {
        await result.current.validate({ email: "invalid" });
      });
      expect(result.current.errors.length).toBeGreaterThan(0);

      act(() => {
        result.current.clearErrors();
      });
      expect(result.current.errors).toEqual([]);
      expect(result.current.getError("email")).toBeUndefined();
    });
  });

  describe("useSetBudgetValidation", () => {
    it("passes validation for valid budget request", async () => {
      const { result } = renderHook(() => useSetBudgetValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          userId: "usr_123",
          limitUsd: 100,
        });
      });

      expect(isValid).toBe(true);
      expect(result.current.errors).toEqual([]);
    });

    it("fails validation when limitUsd is negative", async () => {
      const { result } = renderHook(() => useSetBudgetValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          userId: "usr_123",
          limitUsd: -5,
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.getError("limit_usd")).toBeDefined();
    });

    it("fails validation when userId is empty", async () => {
      const { result } = renderHook(() => useSetBudgetValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          userId: "",
          limitUsd: 50,
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.getError("user_id")).toBeDefined();
    });
  });

  describe("useCreateGrantValidation", () => {
    it("passes validation for valid grant parameters", async () => {
      const { result } = renderHook(() => useCreateGrantValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          userId: "usr_456",
          amountUsd: 50,
          reason: "Model fine-tuning grant",
        });
      });

      expect(isValid).toBe(true);
      expect(result.current.errors).toEqual([]);
    });

    it("fails validation for empty reason or non-positive amount", async () => {
      const { result } = renderHook(() => useCreateGrantValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          userId: "usr_456",
          amountUsd: 0,
          reason: "",
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.errors.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("useCatalogEntryValidation", () => {
    it("passes validation for valid model entry", async () => {
      const { result } = renderHook(() => useCatalogEntryValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          modelId: "gpt-4o",
          provider: "openai",
          displayName: "GPT-4o",
          inputPerMillion: 5.0,
          outputPerMillion: 15.0,
          enabled: true,
        });
      });

      expect(isValid).toBe(true);
      expect(result.current.errors).toEqual([]);
    });

    it("fails validation for missing modelId", async () => {
      const { result } = renderHook(() => useCatalogEntryValidation());

      let isValid: boolean | undefined;
      await act(async () => {
        isValid = await result.current.validate({
          modelId: "",
          provider: "openai",
          displayName: "GPT-4o",
          inputPerMillion: 5.0,
          outputPerMillion: 15.0,
        });
      });

      expect(isValid).toBe(false);
      expect(result.current.getError("model_id")).toBeDefined();
    });
  });
});
