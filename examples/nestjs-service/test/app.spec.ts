import { describe, expect, it } from "vitest";
import { AppController } from "../src/app.controller";

describe("AppController", () => {
  it("returns a root message", () => {
    const controller = new AppController();

    expect(controller.getRoot()).toEqual({
      message: "Hello from TSPack NestJS service example",
    });
  });

  it("returns a health response", () => {
    const controller = new AppController();

    expect(controller.getHealth()).toEqual({
      ok: true,
    });
  });
});
