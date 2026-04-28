// @ts-check
// This file is manually mirrored for the sibling Wails app.

export function GetAppState() {
  return window["go"]["main"]["App"]["GetAppState"]();
}

export function CancelPrescan() {
  return window["go"]["main"]["App"]["CancelPrescan"]();
}

export function InstallDependency(arg1) {
  return window["go"]["main"]["App"]["InstallDependency"](arg1);
}

export function OpenArtifact(arg1, arg2) {
  return window["go"]["main"]["App"]["OpenArtifact"](arg1, arg2);
}

export function PickKeywordsFile() {
  return window["go"]["main"]["App"]["PickKeywordsFile"]();
}

export function PickOutputDirectory() {
  return window["go"]["main"]["App"]["PickOutputDirectory"]();
}

export function PickSourceDirectory() {
  return window["go"]["main"]["App"]["PickSourceDirectory"]();
}

export function PrescanSource(arg1) {
  return window["go"]["main"]["App"]["PrescanSource"](arg1);
}

export function StartRun(arg1) {
  return window["go"]["main"]["App"]["StartRun"](arg1);
}

export function ValidateRun(arg1) {
  return window["go"]["main"]["App"]["ValidateRun"](arg1);
}
