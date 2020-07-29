module.exports = {
    authType: "client-credentials",
    baseURL: `${process.env.BASE_URL}/api/v1`,
    clientId: process.env.CLIENT_ID,
    clientSecret: process.env.CLIENT_SECRET,
    description: "The Beneficiary Claims Data API (BCDA) enables Accountable Care Organizations (ACOs) participating in the Shared Savings Program to retrieve Medicare Part A, Part B, and Part D claims data for their prospectively assigned or assignable beneficiaries.",
    fastestResource: "Patient",
    groupExportEndpoint: "/Group/all/$export",
    jwks: {},
    jwksAuth: false,
    jwksUrl: "http://localhost:3000/jwks",
    jwksUrlAuth: false,
    name: "CMS Beneficiary Claims Data API (BCDA)",
    patientExportEndpoint: "/Patient/$export",
    public: false,
    requiresAuth: true,
    sinceParam: "_since",
    strictSSL: true,
    systemExportEndpoint: "",
    tokenEndpoint: `${process.env.BASE_URL}/auth/token`,
};