import { describe, expect, it } from "vitest";
import { navigateToGoogleWallet } from "@/lib/google-wallet";

describe("Wallet navigation URL validation",()=>{
  for(const url of ["https://evil.example/gp/v/save/a.b.c","javascript:alert(1)","https://pay.google.com.evil.example/gp/v/save/a.b.c","https://user@pay.google.com/gp/v/save/a.b.c","http://pay.google.com/gp/v/save/a.b.c","https://pay.google.com/gp/v/save/a.b.c?redirect=evil","https://pay.google.com/gp/v/save/a.b.c#secret","https://pay.google.com/other"]){
    it(`rejects ${url}`,()=>expect(()=>navigateToGoogleWallet(url)).toThrow());
  }
});
