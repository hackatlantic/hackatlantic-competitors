create table if not exists public.applicants (
  user_id text primary key,
  accepted boolean not null default false,
  qr_code_id text unique,
  full_name text,
  email text,
  school text,
  experience text,
  goals text,
  resume_path text,
  applied_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create or replace function public.set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

drop trigger if exists applicants_set_updated_at on public.applicants;

create trigger applicants_set_updated_at
before update on public.applicants
for each row
execute function public.set_updated_at();

insert into storage.buckets (id, name, public)
values ('resumes', 'resumes', false)
on conflict (id) do nothing;
