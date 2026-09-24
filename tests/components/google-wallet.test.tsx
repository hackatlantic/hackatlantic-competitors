import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EventPass } from "@/components/event-pass";
import { ApiError } from "@/lib/api";

const auth = vi.hoisted(() => ({getToken: vi.fn(), userId: "owner" as string | null, isLoaded: true}));
const api = vi.hoisted(() => ({getAttendeePass: vi.fn(), createGoogleWalletPass: vi.fn()}));
const navigate = vi.hoisted(() => vi.fn());
vi.mock("@clerk/nextjs", () => ({useAuth: () => auth}));
vi.mock("@/lib/api", async(original) => ({...await original<typeof import("@/lib/api")>(), createApiClient: () => api}));
vi.mock("@/lib/google-wallet", () => ({navigateToGoogleWallet: navigate}));
const pass = {id:"test-pass",attendeeId:"attendee",displayName:"Test attendee",status:"active",issuedAt:"2026-09-23T12:00:00Z",qrToken:"qr_v1_test",googleWalletAvailable:true};
const saveUrl = "https://pay.google.com/gp/v/save/test.jwt.signature";

describe("Google Wallet save", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks(); auth.userId="owner";
    api.getAttendeePass.mockResolvedValue(pass);
    api.createGoogleWalletPass.mockResolvedValue({saveUrl});
  });
  it("does not create a save link until the attendee clicks",async()=>{
    render(<EventPass/>);
    const button=await screen.findByRole("button",{name:"Add to Google Wallet"});
    expect(api.createGoogleWalletPass).not.toHaveBeenCalled();
    fireEvent.click(button);
    await waitFor(()=>expect(navigate).toHaveBeenCalledWith(saveUrl));
    expect(api.createGoogleWalletPass).toHaveBeenCalledWith();
    expect(screen.getByRole("img",{name:/QR code/})).toBeTruthy();
  });
  it("hides Wallet when the API has not enabled it",async()=>{
    api.getAttendeePass.mockResolvedValue({...pass,googleWalletAvailable:false});
    render(<EventPass/>); await screen.findByRole("heading",{name:"You’re in."});
    expect(screen.queryByRole("button",{name:"Add to Google Wallet"})).toBeNull();
  });
  it("offers retry without removing the QR on an error",async()=>{
    api.createGoogleWalletPass.mockRejectedValueOnce(new ApiError(503,{code:"wallet_unavailable"}));
    render(<EventPass/>);
    fireEvent.click(await screen.findByRole("button",{name:"Add to Google Wallet"}));
    await screen.findByText(/Google Wallet couldn’t open/);
    expect(screen.getByRole("img",{name:/QR code/})).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Add to Google Wallet"}));
    await waitFor(()=>expect(navigate).toHaveBeenCalledTimes(1));
  });
  it("blocks duplicate clicks and ignores a result after sign-out",async()=>{
    let resolve!: (value:{saveUrl:string})=>void;
    api.createGoogleWalletPass.mockReturnValue(new Promise(done=>{resolve=done;}));
    const view=render(<EventPass/>);
    const button=await screen.findByRole("button",{name:"Add to Google Wallet"});
    fireEvent.click(button);fireEvent.click(button);
    expect(api.createGoogleWalletPass).toHaveBeenCalledTimes(1);
    expect((button as HTMLButtonElement).disabled).toBe(true);
    auth.userId=null; view.rerender(<EventPass/>);
    await act(async()=>resolve({saveUrl}));
    expect(navigate).not.toHaveBeenCalled();
  });
  it("does not redirect a different signed-in account to the previous owner's pass",async()=>{
    let resolve!: (value:{saveUrl:string})=>void;
    api.createGoogleWalletPass.mockReturnValue(new Promise(done=>{resolve=done;}));
    const view=render(<EventPass/>);
    fireEvent.click(await screen.findByRole("button",{name:"Add to Google Wallet"}));
    auth.userId="other-owner";api.getAttendeePass.mockReturnValue(new Promise(()=>{}));
    view.rerender(<EventPass/>);
    await act(async()=>resolve({saveUrl}));
    expect(navigate).not.toHaveBeenCalled();
  });
});
