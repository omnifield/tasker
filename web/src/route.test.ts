import { describe, expect, it } from "vitest";
import { nodeHash, parseHash, wsFromKey, wsHash } from "./route";

describe("wsFromKey", () => {
  it("берёт префикс воркспейса из ключа узла", () => {
    expect(wsFromKey("TSK-34")).toBe("TSK");
    expect(wsFromKey("DEVOPSER-103")).toBe("DEVOPSER");
  });
  it("без дефиса возвращает как есть", () => {
    expect(wsFromKey("TSK")).toBe("TSK");
  });
});

describe("parseHash", () => {
  it("ключ узла → ws (префикс) + key", () => {
    expect(parseHash("#/TSK-34")).toEqual({ ws: "TSK", key: "TSK-34" });
    expect(parseHash("#/DEVOPSER-103")).toEqual({ ws: "DEVOPSER", key: "DEVOPSER-103" });
  });
  it("ключ раздела → только ws", () => {
    expect(parseHash("#/TSK")).toEqual({ ws: "TSK" });
  });
  it("пусто/мусор → {}", () => {
    expect(parseHash("")).toEqual({});
    expect(parseHash("#")).toEqual({});
    expect(parseHash("#/")).toEqual({});
  });
  it("терпит хэш без ведущего слэша", () => {
    expect(parseHash("#TSK-1")).toEqual({ ws: "TSK", key: "TSK-1" });
  });
});

describe("nodeHash/wsHash", () => {
  it("строят канонический хэш", () => {
    expect(nodeHash("TSK-34")).toBe("#/TSK-34");
    expect(wsHash("TSK")).toBe("#/TSK");
  });
  it("parseHash обратен к nodeHash/wsHash", () => {
    expect(parseHash(nodeHash("INFRA-7"))).toEqual({ ws: "INFRA", key: "INFRA-7" });
    expect(parseHash(wsHash("VPN"))).toEqual({ ws: "VPN" });
  });
});
