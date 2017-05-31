-- pg_dump --schema-only --no-owner --table bots_chatbot --table bots_channel --table bots_usercount botbot > sql/schema.sql
--
-- PostgreSQL database dump
--

-- Dumped from database version 9.6.2
-- Dumped by pg_dump version 9.6.2

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

SET search_path = public, pg_catalog;

SET default_tablespace = '';

SET default_with_oids = false;

--
-- Name: bots_channel; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE bots_channel (
    id integer NOT NULL,
    created timestamp with time zone NOT NULL,
    updated timestamp with time zone NOT NULL,
    name character varying(250) NOT NULL,
    slug character varying(50) NOT NULL,
    private_slug character varying(50),
    password character varying(250),
    is_public boolean NOT NULL,
    is_featured boolean NOT NULL,
    fingerprint character varying(36),
    public_kudos boolean NOT NULL,
    notes text NOT NULL,
    chatbot_id integer NOT NULL,
    status character varying(20) NOT NULL
);


--
-- Name: bots_channel_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE bots_channel_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bots_channel_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE bots_channel_id_seq OWNED BY bots_channel.id;


--
-- Name: bots_chatbot; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE bots_chatbot (
    id integer NOT NULL,
    is_active boolean NOT NULL,
    server character varying(100) NOT NULL,
    server_password character varying(100),
    server_identifier character varying(164) NOT NULL,
    nick character varying(64) NOT NULL,
    password character varying(100),
    real_name character varying(250) NOT NULL,
    slug character varying(50) NOT NULL,
    max_channels integer NOT NULL
);


--
-- Name: bots_chatbot_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE bots_chatbot_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bots_chatbot_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE bots_chatbot_id_seq OWNED BY bots_chatbot.id;


--
-- Name: bots_usercount; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE bots_usercount (
    id integer NOT NULL,
    dt date NOT NULL,
    counts integer[],
    channel_id integer NOT NULL
);


--
-- Name: bots_usercount_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE bots_usercount_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bots_usercount_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE bots_usercount_id_seq OWNED BY bots_usercount.id;


--
-- Name: bots_channel id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel ALTER COLUMN id SET DEFAULT nextval('bots_channel_id_seq'::regclass);


--
-- Name: bots_chatbot id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_chatbot ALTER COLUMN id SET DEFAULT nextval('bots_chatbot_id_seq'::regclass);


--
-- Name: bots_usercount id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_usercount ALTER COLUMN id SET DEFAULT nextval('bots_usercount_id_seq'::regclass);


--
-- Name: bots_channel bots_channel_name_2eb90853f7a0f4bc_uniq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel
    ADD CONSTRAINT bots_channel_name_2eb90853f7a0f4bc_uniq UNIQUE (name, chatbot_id);


--
-- Name: bots_channel bots_channel_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel
    ADD CONSTRAINT bots_channel_pkey PRIMARY KEY (id);


--
-- Name: bots_channel bots_channel_private_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel
    ADD CONSTRAINT bots_channel_private_slug_key UNIQUE (private_slug);


--
-- Name: bots_channel bots_channel_slug_7ed9a1a261704004_uniq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel
    ADD CONSTRAINT bots_channel_slug_7ed9a1a261704004_uniq UNIQUE (slug, chatbot_id);


--
-- Name: bots_chatbot bots_chatbot_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_chatbot
    ADD CONSTRAINT bots_chatbot_pkey PRIMARY KEY (id);


--
-- Name: bots_usercount bots_usercount_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_usercount
    ADD CONSTRAINT bots_usercount_pkey PRIMARY KEY (id);


--
-- Name: bots_channel_2dbcba41; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_channel_2dbcba41 ON bots_channel USING btree (slug);


--
-- Name: bots_channel_78239581; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_channel_78239581 ON bots_channel USING btree (chatbot_id);


--
-- Name: bots_channel_private_slug_159f495e180a884e_like; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_channel_private_slug_159f495e180a884e_like ON bots_channel USING btree (private_slug varchar_pattern_ops);


--
-- Name: bots_channel_slug_5af8c5a20a5fbc3a_like; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_channel_slug_5af8c5a20a5fbc3a_like ON bots_channel USING btree (slug varchar_pattern_ops);


--
-- Name: bots_chatbot_2dbcba41; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_chatbot_2dbcba41 ON bots_chatbot USING btree (slug);


--
-- Name: bots_chatbot_slug_58696fc5be763136_like; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_chatbot_slug_58696fc5be763136_like ON bots_chatbot USING btree (slug varchar_pattern_ops);


--
-- Name: bots_usercount_72eb6c85; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bots_usercount_72eb6c85 ON bots_usercount USING btree (channel_id);


--
-- Name: bots_channel bots_channel_chatbot_id_4185ac24f448d436_fk_bots_chatbot_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_channel
    ADD CONSTRAINT bots_channel_chatbot_id_4185ac24f448d436_fk_bots_chatbot_id FOREIGN KEY (chatbot_id) REFERENCES bots_chatbot(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: bots_usercount bots_usercount_channel_id_27c71f9cd90272cf_fk_bots_channel_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY bots_usercount
    ADD CONSTRAINT bots_usercount_channel_id_27c71f9cd90272cf_fk_bots_channel_id FOREIGN KEY (channel_id) REFERENCES bots_channel(id) DEFERRABLE INITIALLY DEFERRED;


--
-- PostgreSQL database dump complete
--
