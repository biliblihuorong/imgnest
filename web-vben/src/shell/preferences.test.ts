import { preferences, updatePreferences } from "@vben/preferences";
import { expect, it } from "vitest";
import { enforcePreferences, NO_COPYRIGHT, appPreferences } from "./preferences";

it("ships no Vben copyright defaults and hides the copyright settings block", () => {
  expect(appPreferences.copyright).toEqual({
    enable: false,
    settingShow: false,
    companyName: "",
    companySiteLink: "",
    date: "",
    icp: "",
    icpLink: "",
  });
});

it("overrides copyright values a browser cached from an older build", () => {
  updatePreferences({
    copyright: { enable: true, settingShow: true, companyName: "Vben", icp: "闽ICP备19024351号" },
  });
  enforcePreferences();
  expect(preferences.copyright).toMatchObject(NO_COPYRIGHT);
});

it("turns off Vben's own preferences drawer so both layouts share one settings dialog", () => {
  expect(appPreferences.app?.enablePreferences).toBe(false);
  updatePreferences({ app: { enablePreferences: true } });
  enforcePreferences();
  expect(preferences.app.enablePreferences).toBe(false);
});
