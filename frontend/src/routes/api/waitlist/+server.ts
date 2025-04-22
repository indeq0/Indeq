import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { MongoClient } from 'mongodb';
import { MONGODB_URI } from '$env/static/private';

// MongoDB connection string - should be in an environment variable
const DB_URI = MONGODB_URI || 'mongodb://localhost:27017';
const DB_NAME = 'indeq';
const COLLECTION_NAME = 'waitlist';

// Create a MongoDB client with proper options
const client = new MongoClient(DB_URI, {
  // Add connection pooling options for better performance
  maxPoolSize: 10,
  minPoolSize: 5
});

// Use a singleton pattern for the database connection
let dbConnection: ReturnType<typeof client.db> | null = null;

// Connect to MongoDB
async function connectToDatabase() {
  if (dbConnection) return dbConnection;

  try {
    await client.connect();
    dbConnection = client.db(DB_NAME);
    return dbConnection;
  } catch (error) {
    console.error('Failed to connect to MongoDB:', error);
    throw error;
  }
}

// Email validation function
function isValidEmail(email: string): boolean {
  // RFC 5322 compliant email regex
  const emailRegex =
    /^(([^<>()\[\]\\.,;:\s@"]+(\.[^<>()\[\]\\.,;:\s@"]+)*)|(".+"))@((\[[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}])|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$/;
  return emailRegex.test(email);
}

export const POST: RequestHandler = async ({ request }) => {
  try {
    const { email } = await request.json();

    // Check if email is provided
    if (!email) {
      return json({ success: false, message: 'Email is required!' }, { status: 400 });
    }

    // Validate email format
    if (!isValidEmail(email)) {
      return json(
        { success: false, message: 'Please provide a valid email address!' },
        { status: 400 }
      );
    }

    // Normalize email to lowercase
    const normalizedEmail = email.toLowerCase();

    // Connect to MongoDB
    const db = await connectToDatabase();
    const collection = db.collection(COLLECTION_NAME);

    // Use updateOne with upsert option to handle race conditions
    // This will either:
    // 1. Insert the document if it doesn't exist
    // 2. Do nothing if it already exists (due to the filter)
    const result = await collection.updateOne(
      { email: normalizedEmail }, // filter
      {
        $setOnInsert: {
          email: normalizedEmail,
          createdAt: new Date(),
          source: 'website'
        }
      },
      { upsert: true } // create if doesn't exist
    );

    // Check if a new document was inserted
    if (result.upsertedCount === 0 && result.matchedCount > 0) {
      // Email already exists
      return json(
        {
          success: false,
          message: 'Email is already on the waitlist! 😊'
        },
        { status: 400 }
      );
    }

    return json({
      success: true,
      message: 'Successfully added to the waitlist! 🎉'
    });
  } catch (error) {
    console.error('Waitlist API error:', error);
    return json(
      {
        success: false,
        message: 'Server error processing your request! 😢'
      },
      { status: 500 }
    );
  }
};

// Close MongoDB connection when the server shuts down
process.on('SIGINT', async () => {
  await client.close();
  process.exit(0);
});
