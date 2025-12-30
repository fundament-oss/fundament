create table "organization"."users" (
    "id" uuid not null default uuidv7(),
    "tenant_id" uuid not null,
    "name" text not null,
    "external_id" text not null,
    "created" timestamp with time zone not null default now()
);


CREATE UNIQUE INDEX users_pk ON organization.users USING btree (id);

CREATE UNIQUE INDEX users_uq_external_id ON organization.users USING btree (external_id);

alter table "organization"."users" add constraint "users_pk" PRIMARY KEY using index "users_pk";

alter table "organization"."users" add constraint "users_created_not_null" NOT NULL created;

alter table "organization"."users" add constraint "users_external_id_not_null" NOT NULL external_id;

alter table "organization"."users" add constraint "users_fk_tenant" FOREIGN KEY (tenant_id) REFERENCES organization.tenants(id);

alter table "organization"."users" add constraint "users_id_not_null" NOT NULL id;

alter table "organization"."users" add constraint "users_name_not_null" NOT NULL name;

alter table "organization"."users" add constraint "users_tenant_id_not_null" NOT NULL tenant_id;

alter table "organization"."users" add constraint "users_uq_external_id" UNIQUE using index "users_uq_external_id";
